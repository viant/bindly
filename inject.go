package bindly

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"sync"

	"github.com/viant/bindly/locator"
	"github.com/viant/bindly/state"
	"github.com/viant/structology"
)

// Inject binds dependencies to the target
func (c *BindingContext[T]) Inject(ctx context.Context, target *T) error {
	targetType := reflect.TypeOf(target)
	bindingType, err := c.getBindingType(ctx, targetType)
	if err != nil {
		return err
	}
	return c.injectBindingType(ctx, bindingType, bindingType.Type.WithValue(target), reflect.ValueOf(target))
}

func (c *BindingContext[T]) injectBindingType(ctx context.Context, bindingType *BindingType, targetState *structology.State, targetValue reflect.Value) error {
	for _, group := range c.orderedBindings(bindingType) { //TODO add concurrency
		for _, binding := range group {
			kind := binding.Kind()
			if c.delayedKinds != nil && c.delayedKinds[kind] {
				continue
			}
			if c.allowedKinds != nil && len(c.allowedKinds) > 0 && !c.allowedKinds[kind] {
				continue
			}
			if err := c.setDestinationValue(ctx, binding, targetState, targetValue); err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *BindingContext[T]) orderedBindings(bindingType *BindingType) []Bindings {
	if bindingType == nil || len(bindingType.Bindings) == 0 {
		return nil
	}
	bindings := make(Bindings, 0)
	for _, group := range bindingType.Bindings {
		bindings = append(bindings, group...)
	}
	sort.SliceStable(bindings, func(i, j int) bool {
		return c.bindingPriority(bindings[i]) < c.bindingPriority(bindings[j])
	})
	return groupBindings(bindings)
}

func (c *BindingContext[T]) bindingPriority(binding *Binding) int {
	if binding == nil || binding.priority != 0 || c == nil || c.injector == nil {
		if binding == nil {
			return 0
		}
		return binding.priority
	}
	provider, ok := c.injector.locators.Lookup(binding.Kind())
	if !ok || provider == nil {
		return 0
	}
	return provider.Priority()
}

func (c *BindingContext[T]) setDestinationValue(ctx context.Context, binding *Binding, destState *structology.State, targetValue reflect.Value) error {
	value, ok, err := c.sourceValue(ctx, binding)
	if err != nil {
		return err
	}
	if ok {

		if binding.selector != nil {
			if err := destState.SetValue(binding.selector.Path(), value); err != nil {
				return err
			}
			return nil
		}
		return binding.setReflectValue(targetValue, value)
	}
	return nil
}

func (c *BindingContext[T]) Value(ctx context.Context, location *state.Location) (interface{}, bool, error) {
	locator, ok := c.injector.locators.Lookup(location.Kind)
	if !ok {
		return nil, false, nil
	}
	aLocator := locator.Locate(c.state)
	if aLocator == nil {
		return nil, false, fmt.Errorf("failed to locate: %v", location)
	}
	return c.value(ctx, location, aLocator)
}

func (c *BindingContext[T]) value(ctx context.Context, location *state.Location, source locator.Locator) (interface{}, bool, error) {
	if scoped, ok := source.(locator.ScopedLocator); ok {
		return scoped.ValueInScope(ctx, c, nil, location.In)
	}
	return source.Value(ctx, nil, location.In)
}

func (c *BindingContext[T]) sourceValue(ctx context.Context, binding *Binding) (interface{}, bool, error) {
	provider, ok := c.injector.locators.Lookup(binding.Kind())
	if !ok {
		return nil, false, fmt.Errorf("failed to lookup locator provider for: %v", binding.location)
	}
	isCacheable := binding.cacheEnabled(provider) && c.valueCache != nil
	cacheKey := binding.cacheKey()
	var locker sync.Locker
	if isCacheable {
		prev, ok := c.valueCache.Get(cacheKey)
		if ok {
			return prev, true, nil
		}
		locker = c.valueCache.lock(cacheKey)
		locker.Lock()
		defer locker.Unlock()
		if prev, ok := c.valueCache.Get(cacheKey); ok {
			return prev, true, nil
		}
	}
	aLocator := provider.Locate(c.state)
	if aLocator == nil {
		if c.state == nil {
			return nil, false, fmt.Errorf("binding provider %q requires source state; use WithSource", binding.Kind())
		}
		return nil, false, fmt.Errorf("failed to locate: %v", binding.location)
	}
	value, ok, err := c.locatorValue(ctx, aLocator, binding.providerType(), binding.location.In)
	if err != nil {
		return nil, false, newBindingError(binding, fmt.Errorf("failed to locate %v: %w", binding.location, err))
	}
	if !ok {
		if binding.DefaultValue != nil {
			value = binding.DefaultValue
			ok = true
		}
	}
	if !ok {
		// honor strictMissing policy for Required bindings
		if c.strictMissing && binding.IsRequired() {
			return nil, false, newBindingError(binding, fmt.Errorf("missing required %s value %q", binding.Kind(), binding.In()))
		}
		return nil, false, nil
	}

	if binding.transformer != nil {
		transformed, err := binding.transformer.Transform(ctx, c, value)
		if err != nil {
			return nil, false, newBindingError(binding, fmt.Errorf("failed to transform value %v: %w", binding.location, err))
		}
		value = transformed
	}
	value, err = convertValue(binding.destinationType(), value)
	if err != nil {
		return nil, false, newBindingError(binding, fmt.Errorf("failed to convert value %v: %w", binding.location, err))
	}

	/*TODO
	- add option for traversing resolved dependency for its own binding
	- add option for creating dependency struct on demand  (with or without singlton option)
	*/

	if isCacheable && ok {
		c.valueCache.Put(cacheKey, value)
	}
	return value, ok, nil
}

func (c *BindingContext[T]) locatorValue(ctx context.Context, source locator.Locator, targetType reflect.Type, name string) (interface{}, bool, error) {
	if scoped, ok := source.(locator.ScopedLocator); ok {
		return scoped.ValueInScope(ctx, c, targetType, name)
	}
	return source.Value(ctx, targetType, name)
}

func (c *BindingContext[T]) getBindingType(ctx context.Context, targetType reflect.Type) (*BindingType, error) {
	targetType, err := planTargetType(targetType)
	if err != nil {
		return nil, err
	}
	bindingType, ok := c.injector.bindingCache.Get(targetType)
	if !ok {
		sType := c.injector.stateType(targetType)
		if bindingType, err = c.injector.buildBindings(ctx, sType); err != nil {
			return nil, err
		}
		c.injector.bindingCache.Put(targetType, bindingType)
	}
	return bindingType, nil
}

func (c *BindingContext[T]) adjustValue(selector *structology.Selector, value interface{}) (interface{}, error) {
	return convertValue(selector.Type(), value)
}
