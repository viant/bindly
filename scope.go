package bindly

import (
	"context"
	"errors"
	"fmt"
	"github.com/viant/bindly/input"
	"reflect"
	"sort"
	"strings"

	"github.com/viant/bindly/locator"
	"github.com/viant/bindly/state"
	"github.com/viant/bindly/xform/conv"
	"github.com/viant/structology"
	xshape "github.com/viant/x/shape"
)

type BindOption func(*bindOptions)
type bindOptions struct {
	groupController ResolutionGroupController
	plan            *Plan
	source          any
	cache           *ValueCache
	observer        BindingObserver
	replay          *ReplayBinding
	resolvedInput   *resolvedInput
	resolvedError   error
	allowedKinds    map[string]bool
	delayedKinds    map[string]bool
	strictMissing   bool
}

func WithPlan(plan *Plan) BindOption   { return func(o *bindOptions) { o.plan = plan } }
func WithSource(source any) BindOption { return func(o *bindOptions) { o.source = source } }

func (i *Injector) ForScope(providers ...locator.Provider) (*Injector, error) {
	if i == nil {
		return nil, fmt.Errorf("parent injector is required")
	}
	child, err := NewInjector(WithProviders(providers...), WithResources(i.resources))
	if err != nil {
		return nil, err
	}
	child.parent = i
	child.transformers = i.transformers
	child.bindingCache = i.bindingCache
	child.structTypeCache = i.structTypeCache
	child.bindingTag = i.bindingTag
	child.xformTag = i.xformTag
	return child, nil
}

func (i *Injector) Bind(ctx context.Context, target any, options ...BindOption) error {
	if i == nil {
		return fmt.Errorf("injector is required")
	}
	settings := &bindOptions{cache: i.valueCache, strictMissing: true}
	for _, option := range options {
		if option != nil {
			option(settings)
		}
	}
	resolved, err := prepareResolvedInput(settings, target)
	if err != nil {
		return err
	}
	if settings.replay != nil && settings.replay.Replay == nil {
		return fmt.Errorf("replay values are required")
	}
	source := settings.source
	if source == nil && settings.plan == nil {
		source = target
	}
	// The injector default cache is for source-independent resolutions. Explicit
	// caches retain the opt-in value-only persistence contract.
	if settings.cache == i.valueCache && (source != nil || settings.observer != nil) {
		settings.cache = nil
	}
	invocation := &invocation{groupController: settings.groupController, injector: i, active: map[string]bool{}, cache: map[resolutionKey]resolution{}, persistent: settings.cache, observer: settings.observer, replay: settings.replay, replayTarget: target, resolved: resolved, resolvedTarget: target, allowedKinds: settings.allowedKinds, delayedKinds: settings.delayedKinds, strictMissing: settings.strictMissing}
	if source != nil {
		invocation.source = structology.NewStateType(reflect.TypeOf(source)).WithValue(source)
		if settings.source != nil {
			sourceType := reflect.TypeOf(source)
			for sourceType.Kind() == reflect.Pointer {
				sourceType = sourceType.Elem()
			}
			if sourceType.Size() != 0 {
				if pointer := invocation.source.StatePtr(); pointer != nil {
					invocation.sourceIdentity = fmt.Sprintf("%p", pointer)
				} else {
					invocation.persistent = nil
				}
			}
		}
	}
	return invocation.bind(ctx, target, settings.plan)
}

type invocation struct {
	groupSource     func() *structology.State
	groupController ResolutionGroupController
	captureSources  bool
	injector        *Injector
	source          *structology.State
	active          map[string]bool
	cache           map[resolutionKey]resolution
	persistent      *ValueCache
	observer        BindingObserver
	replay          *ReplayBinding
	replayTarget    any
	resolved        map[string]any
	resolvedTarget  any
	sourceIdentity  string
	allowedKinds    map[string]bool
	delayedKinds    map[string]bool
	strictMissing   bool
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
		plan, err = s.injector.cachedPlan(targetValue.Type())
		if err != nil {
			return err
		}
	}
	if targetValue.Elem().Type() != plan.target {
		return fmt.Errorf("binding plan targets %v, got %T", plan.target, target)
	}
	if len(plan.groups) != 0 {
		return s.bindWithGroups(ctx, target, plan)
	}
	bindings := append([]BindingSpec(nil), plan.bindings...)
	sort.SliceStable(bindings, func(i, j int) bool {
		left, right := bindings[i].Priority, bindings[j].Priority
		if left == 0 {
			left = s.priority(bindings[i].Location.Kind)
		}
		if right == 0 {
			right = s.priority(bindings[j].Location.Kind)
		}
		return left < right
	})
	replaying := s.replay != nil && s.replay.Replay != nil && s.replayTarget == target
	selectedCount := 0
	var replayAssigned map[string]bool
	if replaying {
		replayAssigned = map[string]bool{}
		if s.replay.Replay.Plan() != plan {
			return fmt.Errorf("replay belongs to a different canonical plan")
		}
		sort.SliceStable(bindings, func(i, j int) bool {
			_, left := s.replay.Replay.plan.preflight[bindings[i].Path]
			_, right := s.replay.Replay.plan.preflight[bindings[j].Path]
			return left && !right
		})
		selectedCount = len(s.replay.Replay.plan.preflight)
	}
	checkpoint := func() error {
		if s.replay.Gate != nil {
			if err := s.replay.Replay.authorize(ctx, target, s.replay.Gate); err != nil {
				return err
			}
		}
		if s.replay.Replay.prepared == nil || s.replay.Gate != nil {
			return s.replay.Replay.capturePrepared(ctx, target, replayAssigned)
		}
		return nil
	}
	for bindingIndex, binding := range bindings {
		if replaying && bindingIndex == selectedCount {
			if err := checkpoint(); err != nil {
				return err
			}
			if s.replay.Only {
				return nil
			}
		}
		if s.delayedKinds[binding.Location.Kind] || (len(s.allowedKinds) > 0 && !s.allowedKinds[binding.Location.Kind]) {
			continue
		}
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
		var result resolution
		var err error
		preparedValue := false
		selectedBinding, selected := BindingSpec{}, false
		if replaying {
			selectedBinding, selected = s.replay.Replay.plan.fields[binding.Path]
		}
		if supplied, ok := s.resolved[binding.Path]; ok && s.resolvedTarget == target {
			result = resolution{value: supplied, found: true}
			preparedValue = true
			sourceType = targetType
		} else if selected {
			if prepared, ok := s.replay.Replay.prepared[binding.Path]; ok {
				result.value, err = (xshape.Runtime{}).CloneValue(prepared.value)
				result.found = prepared.found
				preparedValue = true
				sourceType = targetType
			} else {
				result, err = s.replay.Replay.source(ctx, selectedBinding, sourceType)
			}
		} else {
			result, err = s.resolveBindingResult(ctx, &binding.Location, sourceType, binding.Cacheable, binding.BodyNullPolicy)
		}
		if !preparedValue && err == nil && !result.found && binding.DefaultValue != nil {
			var defaultValue any
			defaultValue, err = (xshape.Runtime{}).CloneValue(binding.DefaultValue)
			if err == nil {
				result.value, err = (conv.ValueConverter{}).Convert(defaultValue, sourceType)
			}
			result.found = true
			result.metadata = nil
		}
		if s.strictMissing && err == nil && (!result.found || nilValue(result.value)) && binding.Required != nil && *binding.Required {
			err = binding.inputError(fmt.Errorf("missing required %s value %q", binding.Location.Kind, binding.Location.In))
		}
		if err != nil {
			return &BindingError{Path: binding.Path, Name: binding.Name, Kind: binding.Location.Kind, In: binding.Location.In, Code: binding.ErrorCode, Message: binding.ErrorMessage, Cause: err}
		}
		if !result.found {
			if err := binding.validateRecordCount(nil); err != nil {
				return &BindingError{Path: binding.Path, Name: binding.Name, Kind: binding.Location.Kind, In: binding.Location.In, Code: binding.ErrorCode, Message: binding.ErrorMessage, Cause: err}
			}
			continue
		}
		if sourceType != nil {
			err = binding.inputError(result.convert(sourceType))
			if err != nil {
				return &BindingError{Path: binding.Path, Name: binding.Name, Kind: binding.Location.Kind, In: binding.Location.In, Code: binding.ErrorCode, Message: binding.ErrorMessage, Cause: err}
			}
		}
		if !preparedValue && binding.Transformer != nil {
			if binding.countsSourceRecords() {
				if err = binding.validateRecordCount(result.value); err != nil {
					return &BindingError{Path: binding.Path, Name: binding.Name, Kind: binding.Location.Kind, In: binding.Location.In, Code: binding.ErrorCode, Message: binding.ErrorMessage, Cause: err}
				}
			}
			result.metadata = nil
			result.value, err = binding.Transformer.Transform(ctx, s, result.value)
			if err != nil {
				return &BindingError{Path: binding.Path, Name: binding.Name, Kind: binding.Location.Kind, In: binding.Location.In, Code: binding.ErrorCode, Message: binding.ErrorMessage, Cause: err}
			}
		}
		err = result.convert(targetType)
		if binding.Transformer == nil {
			err = binding.inputError(err)
		}
		if err != nil {
			return &BindingError{Path: binding.Path, Name: binding.Name, Kind: binding.Location.Kind, In: binding.Location.In, Code: binding.ErrorCode, Message: binding.ErrorMessage, Cause: err}
		}
		if result.value == nil {
			result.value = reflect.Zero(targetType).Interface()
			result.metadata = nil
		}
		if preparedValue || binding.Transformer == nil || !binding.countsSourceRecords() {
			if err = binding.validateRecordCount(result.value); err != nil {
				return &BindingError{Path: binding.Path, Name: binding.Name, Kind: binding.Location.Kind, In: binding.Location.In, Code: binding.ErrorCode, Message: binding.ErrorMessage, Cause: err}
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
		if selected {
			replayAssigned[binding.Path] = true
		}
		if s.observer != nil {
			assigned, ok := plan.fields[binding.Path].Value(targetValue)
			if !ok || !assigned.CanInterface() {
				return &BindingError{Path: binding.Path, Name: binding.Name, Kind: binding.Location.Kind, In: binding.Location.In, Cause: fmt.Errorf("assigned field is not accessible")}
			}
			result.value = assigned.Interface()
			if err = s.observed(ctx, target, binding, result); err != nil {
				return err
			}
		}
	}
	if replaying && selectedCount == len(bindings) {
		return checkpoint()
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
	return s.resolveBindingResult(ctx, location, target, cacheable, "")
}
func (s *invocation) resolveBindingResult(ctx context.Context, location *state.Location, target reflect.Type, cacheable *bool, bodyNullPolicy string) (resolution, error) {
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
		// Policy decoding owns fresh records and cannot reuse strict cache entries.
		if bodyNullPolicy != "" {
			shouldCache = false
		}
		cacheKey := resolutionKey{owner: current, location: *location, target: target}
		// Scope identity qualifies persisted entries. Resolve child providers before
		// considering any cached parent value, so cached defaults cannot mask overrides.
		persistentKey := fmt.Sprintf("%p:%s:%s:%v:%s", current, location.Kind, location.In, target, s.sourceIdentity)
		persistent := s.persistent
		if s.captureSources {
			persistent = nil
		}
		if shouldCache {
			if value, ok := s.cache[cacheKey]; ok {
				return value, nil
			}
			if persistent != nil {
				locker := persistent.lock(persistentKey)
				locker.Lock()
				defer locker.Unlock()
				if value, ok := persistent.Get(persistentKey); ok {
					result := resolution{value: value, found: true}
					if err := result.unwrap(false); err != nil {
						return resolution{}, err
					}
					return result, nil
				}
			}
		}
		sourceState := s.source
		if s.groupSource != nil {
			sourceState = s.groupSource()
		}
		valueLocator := provider.Locate(sourceState)
		if valueLocator == nil {
			if s.source == nil {
				return resolution{}, fmt.Errorf("binding provider %q requires source state; use WithSource", location.Kind)
			}
			return resolution{}, fmt.Errorf("provider %s returned no locator", location.Kind)
		}
		var value any
		var found bool
		var err error
		var raw locator.SourceCapturer
		if s.captureSources {
			raw, _ = valueLocator.(locator.SourceCapturer)
		}
		if raw != nil {
			value, found, err = raw.CaptureSource(ctx, target, location.In)
		} else if bodyNullPolicy != "" {
			policyLocator, ok := valueLocator.(locator.BodyNullPolicyLocator)
			if !ok {
				return resolution{}, fmt.Errorf("provider %s does not support bodyNullPolicy", location.Kind)
			}
			value, found, err = policyLocator.ValueWithBodyNullPolicy(ctx, target, location.In, bodyNullPolicy)
		} else if scoped, ok := valueLocator.(locator.ScopedLocator); ok {
			value, found, err = scoped.ValueInScope(ctx, s, target, location.In)
		} else {
			value, found, err = valueLocator.Value(ctx, target, location.In)
		}
		authoritative, _ := valueLocator.(locator.AuthoritativeLocator)
		if found || err != nil || (authoritative != nil && authoritative.Owns(location.In)) {
			result := resolution{value: value, found: found}
			if unwrapErr := result.unwrap(s.MetadataRequested()); unwrapErr != nil {
				return resolution{}, errors.Join(err, unwrapErr)
			}
			if shouldCache && err == nil {
				if s.cache == nil {
					s.cache = map[resolutionKey]resolution{}
				}
				s.cache[cacheKey] = result
				if persistent != nil && found {
					persistent.Put(persistentKey, result.value)
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
	Name    string
	Kind    string
	In      string
	Code    int
	Message string
	Cause   error
	Err     error
}

func (e *BindingError) Error() string {
	if e.Message != "" {
		cause := e.Unwrap()
		var invalid *input.Error
		if errors.As(cause, &invalid) {
			cause = invalid.Cause
		}
		return strings.ReplaceAll(e.Message, "${error}", fmt.Sprint(cause))
	}
	var invalid *input.Error
	if errors.As(e.Unwrap(), &invalid) {
		return invalid.Error()
	}
	return fmt.Sprintf("bind %s: %v", e.Path, e.Unwrap())
}
func (e *BindingError) Unwrap() error {
	if e == nil {
		return nil
	}
	if e.Cause != nil {
		return e.Cause
	}
	return e.Err
}
func (e *BindingError) StatusCode() int {
	if e.Code != 0 {
		return e.Code
	}
	var coder interface{ StatusCode() int }
	if errors.As(e.Unwrap(), &coder) {
		return coder.StatusCode()
	}
	return 0
}

// Only external data-point kinds acquire client-error semantics. Internal
// providers, defaults and transformers retain their own failure contracts.
func (b BindingSpec) inputError(err error) error {
	if err == nil {
		return nil
	}
	switch b.Location.Kind {
	case "query", "path", "header", "cookie", "form", "body":
		return &input.Error{Cause: err}
	}
	return err
}
