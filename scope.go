package bindly

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/viant/bindly/locator"
	"github.com/viant/bindly/state"
	"github.com/viant/bindly/xform/conv"
	"github.com/viant/structology"
)

type BindOption func(*bindOptions)
type bindOptions struct {
	plan     *Plan
	source   any
	cache    *ValueCache
	observer BindingObserver
}

func WithPlan(plan *Plan) BindOption   { return func(o *bindOptions) { o.plan = plan } }
func WithSource(source any) BindOption { return func(o *bindOptions) { o.source = source } }

func (i *Injector) ForScope(providers ...locator.Provider) (*Injector, error) {
	child, err := NewInjector(WithProviders(providers...), WithResources(i.resources))
	if err != nil {
		return nil, err
	}
	child.parent = i
	child.transformers = i.transformers
	child.bindingTag = i.bindingTag
	child.xformTag = i.xformTag
	return child, nil
}

func (i *Injector) Bind(ctx context.Context, target any, options ...BindOption) error {
	settings := &bindOptions{}
	for _, option := range options {
		option(settings)
	}
	source := settings.source
	if source == nil {
		source = target
	}
	invocation := &invocation{injector: i, active: map[string]bool{}, cache: map[resolutionKey]resolution{}, persistent: settings.cache, observer: settings.observer}
	if source != nil {
		invocation.source = structology.NewStateType(reflect.TypeOf(source)).WithValue(source)
	}
	return invocation.bind(ctx, target, settings.plan)
}

type invocation struct {
	injector   *Injector
	source     *structology.State
	active     map[string]bool
	cache      map[resolutionKey]resolution
	persistent *ValueCache
	observer   BindingObserver
}

type resolutionKey struct {
	owner    *Injector
	location state.Location
	target   reflect.Type
}
type resolution struct {
	value    any
	found    bool
	metadata any
}

func (s *invocation) Bind(ctx context.Context, target any) error { return s.bind(ctx, target, nil) }

func (s *invocation) BindTarget(ctx context.Context, target any) error {
	return s.bind(ctx, target, nil)
}
func (s *invocation) bind(ctx context.Context, target any, plan *Plan) error {
	targetValue := reflect.ValueOf(target)
	if !targetValue.IsValid() || targetValue.Kind() != reflect.Pointer || targetValue.IsNil() {
		return fmt.Errorf("binding target must be a nonnil pointer")
	}
	var err error
	if plan == nil {
		plan, err = s.injector.CompilePlan(targetValue.Type())
		if err != nil {
			return err
		}
	}
	if targetValue.Elem().Type() != plan.target {
		return fmt.Errorf("binding plan targets %v, got %T", plan.target, target)
	}
	bindings := append([]BindingSpec(nil), plan.bindings...)
	sort.SliceStable(bindings, func(i, j int) bool {
		return s.priority(bindings[i].Location.Kind) < s.priority(bindings[j].Location.Kind)
	})
	for _, binding := range bindings {
		if binding.When != "" {
			if s.source == nil {
				return fmt.Errorf("binding %s condition requires source state", binding.Path)
			}
			condition, err := s.source.Value(strings.TrimPrefix(binding.When, "$"))
			if err != nil {
				return err
			}
			enabled, ok := condition.(bool)
			if !ok {
				return fmt.Errorf("binding %s condition must resolve to bool", binding.Path)
			}
			if !enabled {
				continue
			}
		}
		targetType := plan.fields[binding.Path].Type
		sourceType := binding.SourceType
		if sourceType == nil {
			sourceType = targetType
			if binding.Transformer != nil {
				sourceType = nil
			}
		}
		result, err := s.resolveResult(ctx, &binding.Location, sourceType, binding.Cacheable)
		if err == nil && !result.found && binding.DefaultValue != nil {
			result.value, err = (conv.ValueConverter{}).Convert(binding.DefaultValue, sourceType)
			result.found = true
			result.metadata = nil
		}
		if err == nil && (!result.found || nilValue(result.value)) && binding.Required != nil && *binding.Required {
			err = fmt.Errorf("missing required %s value %q", binding.Location.Kind, binding.Location.In)
		}
		if err != nil {
			return &BindingError{Path: binding.Path, Code: binding.ErrorCode, Message: binding.ErrorMessage, Cause: err}
		}
		if !result.found {
			if err := binding.validateRecordCount(nil); err != nil {
				return &BindingError{Path: binding.Path, Code: binding.ErrorCode, Message: binding.ErrorMessage, Cause: err}
			}
			continue
		}
		if sourceType != nil {
			err = result.convert(sourceType)
			if err != nil {
				return &BindingError{Path: binding.Path, Code: binding.ErrorCode, Message: binding.ErrorMessage, Cause: err}
			}
		}
		if binding.Transformer != nil {
			if binding.countsSourceRecords() {
				if err = binding.validateRecordCount(result.value); err != nil {
					return &BindingError{Path: binding.Path, Code: binding.ErrorCode, Message: binding.ErrorMessage, Cause: err}
				}
			}
			result.metadata = nil
			result.value, err = binding.Transformer.Transform(ctx, s, result.value)
			if err != nil {
				return &BindingError{Path: binding.Path, Code: binding.ErrorCode, Message: binding.ErrorMessage, Cause: err}
			}
		}
		err = result.convert(targetType)
		if err != nil {
			return &BindingError{Path: binding.Path, Code: binding.ErrorCode, Message: binding.ErrorMessage, Cause: err}
		}
		if result.value == nil {
			result.value = reflect.Zero(targetType).Interface()
			result.metadata = nil
		}
		if binding.Transformer == nil || !binding.countsSourceRecords() {
			if err = binding.validateRecordCount(result.value); err != nil {
				return &BindingError{Path: binding.Path, Code: binding.ErrorCode, Message: binding.ErrorMessage, Cause: err}
			}
		}
		if err = plan.fields[binding.Path].Set(targetValue, result.value); err != nil {
			return err
		}
		if binding.MarkerField != "" {
			if err = plan.fields[binding.MarkerField].Set(targetValue, true); err != nil {
				return err
			}
		}
		if s.observer != nil {
			assigned, ok := plan.fields[binding.Path].Value(targetValue)
			if !ok || !assigned.CanInterface() {
				return &BindingError{Path: binding.Path, Cause: fmt.Errorf("assigned field is not accessible")}
			}
			result.value = assigned.Interface()
			if err = s.observed(ctx, target, binding, result); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *invocation) priority(kind string) int {
	for current := s.injector; current != nil; current = current.parent {
		if provider, ok := current.locators.Lookup(kind); ok {
			return provider.Priority()
		}
	}
	return 0
}
func (s *invocation) Value(ctx context.Context, location *state.Location) (any, bool, error) {
	return s.resolve(ctx, location, nil)
}
func (s *invocation) resolve(ctx context.Context, location *state.Location, target reflect.Type) (any, bool, error) {
	return s.resolveWithPolicy(ctx, location, target, nil)
}

func (s *invocation) resolveWithPolicy(ctx context.Context, location *state.Location, target reflect.Type, cacheable *bool) (any, bool, error) {
	result, err := s.resolveResult(ctx, location, target, cacheable)
	return result.value, result.found, err
}

func (s *invocation) resolveResult(ctx context.Context, location *state.Location, target reflect.Type, cacheable *bool) (resolution, error) {
	if location == nil {
		return resolution{}, fmt.Errorf("binding location is required")
	}
	key := location.Kind + ":" + location.In
	if s.active[key] {
		return resolution{}, fmt.Errorf("cyclic binding dependency %s", key)
	}
	s.active[key] = true
	defer delete(s.active, key)
	for current := s.injector; current != nil; current = current.parent {
		provider, ok := current.locators.Lookup(location.Kind)
		if !ok {
			continue
		}
		shouldCache := false
		if policy, ok := provider.(locator.CachePolicy); ok {
			shouldCache = policy.DefaultCacheable()
		}
		if cacheable != nil {
			shouldCache = *cacheable
		}
		cacheKey := resolutionKey{owner: current, location: *location, target: target}
		// Scope identity qualifies persisted entries. Resolve child providers before
		// considering any cached parent value, so cached defaults cannot mask overrides.
		persistentKey := fmt.Sprintf("%p:%s:%s:%v", current, location.Kind, location.In, target)
		if shouldCache {
			if value, ok := s.cache[cacheKey]; ok {
				return value, nil
			}
			if s.persistent != nil {
				if value, ok := s.persistent.Get(persistentKey); ok {
					result := resolution{value: value, found: true}
					if err := result.unwrap(false); err != nil {
						return resolution{}, err
					}
					return result, nil
				}
			}
		}
		valueLocator := provider.Locate(s.source)
		if valueLocator == nil {
			return resolution{}, fmt.Errorf("provider %s returned no locator", location.Kind)
		}
		var value any
		var found bool
		var err error
		if scoped, ok := valueLocator.(locator.ScopedLocator); ok {
			value, found, err = scoped.ValueInScope(ctx, s, target, location.In)
		} else {
			value, found, err = valueLocator.Value(ctx, target, location.In)
		}
		if found || err != nil {
			result := resolution{value: value, found: found}
			if unwrapErr := result.unwrap(s.MetadataRequested()); unwrapErr != nil {
				return resolution{}, errors.Join(err, unwrapErr)
			}
			if shouldCache && err == nil {
				if s.cache == nil {
					s.cache = map[resolutionKey]resolution{}
				}
				s.cache[cacheKey] = result
				if s.persistent != nil && found {
					s.persistent.Put(persistentKey, result.value)
				}
			}
			if err != nil {
				result.metadata = nil
			}
			return result, err
		}
	}
	return resolution{}, nil
}

func nilValue(value any) bool {
	if value == nil {
		return true
	}
	actual := reflect.ValueOf(value)
	switch actual.Kind() {
	case reflect.Pointer, reflect.Map, reflect.Slice, reflect.Interface, reflect.Func, reflect.Chan:
		return actual.IsNil()
	}
	return false
}

type BindingError struct {
	Path    string
	Code    int
	Message string
	Cause   error
}

func (e *BindingError) Error() string {
	if e.Message != "" {
		return strings.ReplaceAll(e.Message, "${error}", fmt.Sprint(e.Cause))
	}
	return fmt.Sprintf("bind %s: %v", e.Path, e.Cause)
}
func (e *BindingError) Unwrap() error   { return e.Cause }
func (e *BindingError) StatusCode() int { return e.Code }
