package bindly

import (
	"context"
	"fmt"
	"reflect"

	"github.com/viant/bindly/state"
	"github.com/viant/bindly/xform/conv"
	"github.com/viant/structology"
)

// DynamicContext binds runtime-owned targets through the canonical invocation.
type DynamicContext struct {
	injector   *Injector
	state      *structology.State
	valueCache *ValueCache
}

func WithDynamicState(injector *Injector, stateValue interface{}) *DynamicContext {
	if stateValue == nil {
		stateValue = &struct{}{}
	}
	reflectType := reflect.TypeOf(stateValue)
	structType, ok := injector.structTypeCache.Get(reflectType)
	if !ok {
		structType = structology.NewStateType(reflectType)
		injector.structTypeCache.Put(reflectType, structType)
	}
	return &DynamicContext{injector: injector, state: structType.WithValue(stateValue), valueCache: NewValueCache()}
}

func (c *DynamicContext) Inject(ctx context.Context, target interface{}) error {
	if target == nil {
		return nil
	}
	source := c.state.StatePtr()
	if source == nil {
		source = c.state.State()
	}
	return c.injector.Bind(ctx, target, WithSource(source), func(options *bindOptions) { options.cache = c.valueCache })
}

// Assign resolves a typed location and writes its value into a nonnil pointer.
func (c *DynamicContext) Assign(ctx context.Context, target interface{}, location *state.Location) error {
	if target == nil {
		return nil
	}
	destination := reflect.ValueOf(target)
	if destination.Kind() != reflect.Pointer || destination.IsNil() {
		return fmt.Errorf("assign requires non-nil pointer target")
	}
	targetType := destination.Elem().Type()
	value, found, err := c.invocation().resolve(ctx, location, targetType)
	if err != nil || !found {
		return err
	}
	converted, err := (conv.ValueConverter{}).Convert(value, targetType)
	if err != nil {
		return err
	}
	if converted == nil {
		destination.Elem().Set(reflect.Zero(targetType))
	} else {
		destination.Elem().Set(reflect.ValueOf(converted))
	}
	return nil
}

func (c *DynamicContext) Value(ctx context.Context, location *state.Location) (interface{}, bool, error) {
	return c.invocation().Value(ctx, location)
}

func (c *DynamicContext) invocation() *invocation {
	return &invocation{injector: c.injector, source: c.state, active: map[string]bool{}, cache: map[resolutionKey]resolution{}, persistent: c.valueCache}
}
