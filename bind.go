package bindly

import (
	"context"
	"fmt"
	"reflect"

	"github.com/viant/structology"
)

// Bind injects values into any non-nil pointer to a struct. Callers may supply
// an explicit compiled Plan; without one, Bind uses the injector's cached tag
// plan for the target type.
func (i *Injector) Bind(ctx context.Context, target interface{}, opts ...BindOption) error {
	if i == nil {
		return fmt.Errorf("injector is required")
	}
	targetValue := reflect.ValueOf(target)
	if !targetValue.IsValid() || targetValue.Kind() != reflect.Ptr || targetValue.IsNil() {
		return fmt.Errorf("binding target must be a non-nil pointer, got %T", target)
	}
	targetType, err := planTargetType(targetValue.Type())
	if err != nil {
		return err
	}
	options := bindOptions{strictMissing: true, cache: i.valueCache}
	for _, option := range opts {
		if option != nil {
			option(&options)
		}
	}

	bindingType, err := i.bindingType(ctx, targetType, options.plan)
	if err != nil {
		return err
	}
	var sourceState *structology.State
	if options.source != nil {
		sourceState = i.stateFor(options.source)
	}
	var targetState *structology.State
	if options.plan == nil {
		targetState = i.stateType(targetType).WithValue(target)
		if sourceState == nil {
			sourceState = targetState
		}
	}
	bindingContext := &BindingContext[interface{}]{
		injector:      i,
		state:         sourceState,
		valueCache:    options.cache,
		allowedKinds:  options.allowedKinds,
		delayedKinds:  options.delayedKinds,
		strictMissing: options.strictMissing,
	}
	return bindingContext.injectBindingType(ctx, bindingType, targetState, targetValue)
}

func (i *Injector) bindingType(ctx context.Context, targetType reflect.Type, plan *Plan) (*BindingType, error) {
	if plan == nil {
		context := &BindingContext[interface{}]{injector: i}
		return context.getBindingType(ctx, targetType)
	}
	if plan.targetType != targetType {
		return nil, fmt.Errorf("binding plan targets %s, not %s", plan.targetType, targetType)
	}
	return plan.bindingType, nil
}

func (i *Injector) stateType(rType reflect.Type) *structology.StateType {
	if stateType, ok := i.structTypeCache.Get(rType); ok {
		return stateType
	}
	stateType := structology.NewStateType(rType)
	i.structTypeCache.Put(rType, stateType)
	return stateType
}

func (i *Injector) stateFor(value interface{}) *structology.State {
	rType := reflect.TypeOf(value)
	return i.stateType(rType).WithValue(value)
}
