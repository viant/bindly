package bindly

import (
	"context"
	"fmt"
	"reflect"

	"github.com/viant/structology"
)

// BindingContext represents binding context with state
type BindingContext[T any] struct {
	injector   *Injector
	state      *structology.State
	bindings   []Bindings
	valueCache *ValueCache
	// per-bind options
	allowedKinds  map[string]bool // only these kinds are executed if set
	delayedKinds  map[string]bool // kinds to skip in this run
	strictMissing bool            // whether missing Required values cause error
}

func (c *BindingContext[T]) Bind(ctx context.Context) error {
	// reuse structology state type from the current state
	stateType := c.state.Type()
	targetType := stateType.Type()
	typeBinding, err := c.getBindingType(ctx, targetType)
	if err != nil {
		return err
	}
	for _, group := range c.orderedBindings(typeBinding) { // TODO add concurrency
		for _, binding := range group {
			kind := binding.Kind()
			if c.delayedKinds != nil && c.delayedKinds[kind] {
				continue
			}
			if c.allowedKinds != nil && len(c.allowedKinds) > 0 && !c.allowedKinds[kind] {
				continue
			}
			if err := c.setDestinationValue(ctx, binding, c.state, reflect.Value{}); err != nil {
				return err
			}
		}
	}
	return nil
}

// BindTarget binds a nested target from the same source state and provider
// scope as the current binding operation.
func (c *BindingContext[T]) BindTarget(ctx context.Context, target interface{}) error {
	if c == nil || c.injector == nil {
		return fmt.Errorf("binding scope is required")
	}
	targetValue := reflect.ValueOf(target)
	if !targetValue.IsValid() || targetValue.Kind() != reflect.Ptr || targetValue.IsNil() {
		return fmt.Errorf("binding target must be a non-nil pointer, got %T", target)
	}
	targetType, err := planTargetType(targetValue.Type())
	if err != nil {
		return err
	}
	bindingType, err := c.getBindingType(ctx, targetType)
	if err != nil {
		return err
	}
	targetState := c.injector.stateType(targetType).WithValue(target)
	return c.injectBindingType(ctx, bindingType, targetState, targetValue)
}

func WithState[T any](binder *Injector, state interface{}, opt ...BindingOption[T]) *BindingContext[T] {
	reflectType := reflect.TypeOf(state)
	structType, ok := binder.structTypeCache.Get(reflectType)
	if !ok {
		structType = structology.NewStateType(reflectType)
		binder.structTypeCache.Put(reflectType, structType)
	}
	stateValue := structType.WithValue(state)
	ret := &BindingContext[T]{injector: binder, state: stateValue, valueCache: binder.valueCache, strictMissing: true}
	for _, o := range opt {
		o(ret)
	}
	return ret
}
