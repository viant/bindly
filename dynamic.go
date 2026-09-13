package bindly

import (
	"context"
	"fmt"
	"github.com/viant/bindly/locator"
	"github.com/viant/bindly/state"
	"github.com/viant/structology"
	"reflect"
	"sync"
)

// DynamicContext provides a non-generic binding path for runtime-owned targets.
// It exists so adapter layers can use bindly behind narrow public contracts
// without needing compile-time knowledge of the target type.
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
	return &DynamicContext{
		injector:   injector,
		state:      structType.WithValue(stateValue),
		valueCache: NewValueCache(),
	}
}

func (c *DynamicContext) Inject(ctx context.Context, target interface{}) error {
	if target == nil {
		return nil
	}
	targetType := reflect.TypeOf(target)
	if targetType.Kind() != reflect.Ptr || reflect.ValueOf(target).IsNil() {
		return fmt.Errorf("inject requires non-nil pointer target")
	}
	bindingType, err := c.getBindingType(ctx, targetType)
	if err != nil {
		return err
	}
	targetState := bindingType.Type.WithValue(target)
	for _, group := range bindingType.Bindings {
		for _, binding := range group {
			if err := c.setDestinationValue(ctx, binding, targetState); err != nil {
				return err
			}
		}
	}
	return nil
}

// Assign resolves a location and writes its value into target.
// target must be a non-nil pointer to the destination value.
func (c *DynamicContext) Assign(ctx context.Context, target interface{}, location *state.Location) error {
	if target == nil {
		return nil
	}
	targetValue := reflect.ValueOf(target)
	if targetValue.Kind() != reflect.Ptr || targetValue.IsNil() {
		return fmt.Errorf("assign requires non-nil pointer target")
	}
	if location == nil {
		return fmt.Errorf("assign location was nil")
	}
	value, ok, err := c.Value(ctx, location)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	adjusted, err := c.adjustElementValue(targetValue.Elem().Type(), value)
	if err != nil {
		return err
	}
	targetValue.Elem().Set(reflect.ValueOf(adjusted))
	return nil
}

func (c *DynamicContext) setDestinationValue(ctx context.Context, binding *Binding, destState *structology.State) error {
	value, ok, err := c.sourceValue(ctx, binding)
	if err != nil {
		return err
	}
	if ok {
		if err := destState.SetValue(binding.selector.Path(), value); err != nil {
			return err
		}
	}
	return nil
}

func (c *DynamicContext) Value(ctx context.Context, location *state.Location) (interface{}, bool, error) {
	locatorItem, ok := c.injector.locators.Get(location.Kind)
	if !ok {
		return nil, false, fmt.Errorf("failed to lookup locator for: %v", location.Kind)
	}
	aLocator := locatorItem.Locate(c.state)
	if aLocator == nil {
		return nil, false, fmt.Errorf("failed to locate: %v", location)
	}
	return c.value(ctx, location, aLocator)
}

func (c *DynamicContext) value(ctx context.Context, location *state.Location, aLocator locator.Locator) (interface{}, bool, error) {
	return aLocator.Value(ctx, location.In)
}

func (c *DynamicContext) sourceValue(ctx context.Context, binding *Binding) (interface{}, bool, error) {
	isCacheable := binding.cachable && c.valueCache != nil
	cacheKey := binding.selector.Path()
	if binding.location != nil {
		cacheKey = binding.location.Kind + ":" + binding.location.In
	}
	var locker sync.Locker
	if isCacheable {
		prev, ok := c.valueCache.Get(cacheKey)
		if ok {
			return prev, true, nil
		}
		locker = c.valueCache.lock(cacheKey)
		locker.Lock()
		defer locker.Unlock()
	}
	aLocator := binding.provider.Locate(c.state)
	if aLocator == nil {
		return nil, false, fmt.Errorf("failed to locate: %v", binding.location)
	}
	value, ok, err := c.value(ctx, binding.location, aLocator)
	if err != nil {
		return nil, false, fmt.Errorf("failed to locate: %v, %w", binding.location, err)
	}
	if !ok && binding.defaultValue != nil {
		value = binding.defaultValue
		ok = true
	}
	if !ok {
		if binding.required {
			return nil, false, fmt.Errorf("required value not found: %+v", binding.location)
		}
		return nil, false, nil
	}
	value, err = c.adjustValue(binding.selector, value)
	if err != nil {
		return nil, false, fmt.Errorf("failed to adjust value: %v, %w", binding.location, err)
	}
	if binding.transformer != nil {
		transformed, err := binding.transformer.Transform(ctx, c, value)
		if err != nil {
			return nil, false, fmt.Errorf("failed to transform value: %v, %w", binding.location, err)
		}
		value = transformed
	}
	if isCacheable && ok {
		c.valueCache.Put(cacheKey, value)
	}
	return value, ok, nil
}

func (c *DynamicContext) getBindingType(ctx context.Context, targetType reflect.Type) (*BindingType, error) {
	bindingType, ok := c.injector.bindingCache.Get(targetType)
	if !ok {
		var err error
		sType := structology.NewStateType(targetType)
		if bindingType, err = c.injector.buildBindings(ctx, sType); err != nil {
			return nil, err
		}
		c.injector.bindingCache.Put(targetType, bindingType)
	}
	return bindingType, nil
}

func (c *DynamicContext) adjustValue(selector *structology.Selector, value interface{}) (interface{}, error) {
	if value == nil {
		return nil, nil
	}

	selectorType := selector.Type()
	valueType := reflect.TypeOf(value)

	if valueType.AssignableTo(selectorType) {
		return value, nil
	}
	if selectorType.Kind() == reflect.Ptr && valueType.Kind() != reflect.Ptr {
		if !valueType.AssignableTo(selectorType.Elem()) {
			return nil, fmt.Errorf("incompatible types: selector expects %v but got %v", selectorType, valueType)
		}
		valueReflect := reflect.ValueOf(value)
		ptrValue := reflect.New(valueType)
		ptrValue.Elem().Set(valueReflect)
		return ptrValue.Interface(), nil
	}
	if selectorType.Kind() != reflect.Ptr && valueType.Kind() == reflect.Ptr {
		if !valueType.Elem().AssignableTo(selectorType) {
			return nil, fmt.Errorf("incompatible types: selector expects %v but got %v", selectorType, valueType)
		}
		valueReflect := reflect.ValueOf(value)
		if valueReflect.IsNil() {
			return reflect.Zero(selectorType).Interface(), nil
		}
		return valueReflect.Elem().Interface(), nil
	}
	if selectorType.Kind() == reflect.Slice && valueType.Kind() == reflect.Slice {
		return c.adjustSliceValue(selectorType, value)
	}
	return nil, fmt.Errorf("incompatible types: selector expects %v but got %v", selectorType, valueType)
}

func (c *DynamicContext) adjustSliceValue(selectorType reflect.Type, value interface{}) (interface{}, error) {
	valueSlice := reflect.ValueOf(value)
	length := valueSlice.Len()
	elemType := selectorType.Elem()
	resultSlice := reflect.MakeSlice(selectorType, length, length)
	for i := 0; i < length; i++ {
		elem := valueSlice.Index(i).Interface()
		adjustedElem, err := c.adjustElementValue(elemType, elem)
		if err != nil {
			return nil, fmt.Errorf("error converting slice element at index %d: %w", i, err)
		}
		resultSlice.Index(i).Set(reflect.ValueOf(adjustedElem))
	}
	return resultSlice.Interface(), nil
}

func (c *DynamicContext) adjustElementValue(targetType reflect.Type, value interface{}) (interface{}, error) {
	if value == nil {
		return reflect.Zero(targetType).Interface(), nil
	}
	valueType := reflect.TypeOf(value)
	valueReflect := reflect.ValueOf(value)
	if valueType.AssignableTo(targetType) {
		return value, nil
	}
	if targetType.Kind() == reflect.Ptr && valueType.Kind() != reflect.Ptr {
		if !valueType.AssignableTo(targetType.Elem()) {
			return nil, fmt.Errorf("incompatible element types: target expects %v but got %v", targetType, valueType)
		}
		ptrValue := reflect.New(valueType)
		ptrValue.Elem().Set(valueReflect)
		return ptrValue.Interface(), nil
	}
	if targetType.Kind() != reflect.Ptr && valueType.Kind() == reflect.Ptr {
		if !valueType.Elem().AssignableTo(targetType) {
			return nil, fmt.Errorf("incompatible element types: target expects %v but got %v", targetType, valueType)
		}
		if valueReflect.IsNil() {
			return reflect.Zero(targetType).Interface(), nil
		}
		return valueReflect.Elem().Interface(), nil
	}
	if isNumericType(targetType) && isNumericType(valueType) {
		return convertNumeric(targetType, valueReflect)
	}
	if targetType.Kind() == reflect.String {
		return fmt.Sprintf("%v", value), nil
	}
	return nil, fmt.Errorf("incompatible element types: target expects %v but got %v", targetType, valueType)
}
