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

// Inject binds dependencies to the target
func (c *BindingContext[T]) Inject(ctx context.Context, target *T) error {
	targetType := reflect.TypeOf(target)
	bindingType, err := c.getBindingType(ctx, targetType)
	if err != nil {
		return err
	}
	targetState := bindingType.Type.WithValue(target)
	for _, group := range bindingType.Bindings { //TODO add concurrency
		for _, binding := range group {
			if err := c.setDestinationValue(ctx, binding, targetState); err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *BindingContext[T]) setDestinationValue(ctx context.Context, binding *Binding, destState *structology.State) error {
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

func (c *BindingContext[T]) Value(ctx context.Context, location *state.Location) (interface{}, bool, error) {
	locator, ok := c.injector.locators.Get(location.Kind)
	if !ok {
		return nil, false, fmt.Errorf("failed to lookup locator for: %v", location.Kind)
	}
	aLocator := locator.Locate(c.state)
	if aLocator == nil {
		return nil, false, fmt.Errorf("failed to locate: %v", location)
	}
	return c.value(ctx, location, aLocator)
}

func (c *BindingContext[T]) value(ctx context.Context, location *state.Location, locator locator.Locator) (interface{}, bool, error) {
	return locator.Value(ctx, location.In)
}

func (c *BindingContext[T]) sourceValue(ctx context.Context, binding *Binding) (interface{}, bool, error) {
	isCacheable := binding.cachable && c.valueCache != nil
	aPath := binding.selector.Path()
	var locker sync.Locker
	if isCacheable {
		prev, ok := c.valueCache.Get(aPath)
		if ok {
			return prev, true, nil
		}
		locker = c.valueCache.lock(aPath)
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
	if !ok {
		if binding.defaultValue != nil {
			value = binding.defaultValue
			ok = true
		}
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

	/*TODO
		- add option for traversing resolved dependency for its own binding
		- add option for creating dependency struct on demand  (with or without singlton option)
	*/

	if isCacheable && ok {
		c.valueCache.Put(aPath, value)
	}
	return value, ok, nil
}

func (c *BindingContext[T]) getBindingType(ctx context.Context, targetType reflect.Type) (*BindingType, error) {
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

// adjustValue ensures type compatibility between the selector and value
func (c *BindingContext[T]) adjustValue(selector *structology.Selector, value interface{}) (interface{}, error) {
	if value == nil {
		return nil, nil
	}

	selectorType := selector.Type()
	valueType := reflect.TypeOf(value)

	// If types are already compatible, return as is
	if valueType.AssignableTo(selectorType) {
		return value, nil
	}

	// Handle special case: pointer vs. non-pointer
	if selectorType.Kind() == reflect.Ptr && valueType.Kind() != reflect.Ptr {
		// Need to convert non-pointer value to pointer
		if !valueType.AssignableTo(selectorType.Elem()) {
			return nil, fmt.Errorf("incompatible types: selector expects %v but got %v", selectorType, valueType)
		}
		valueReflect := reflect.ValueOf(value)
		ptrValue := reflect.New(valueType)
		ptrValue.Elem().Set(valueReflect)
		return ptrValue.Interface(), nil
	}

	// If selector is expecting non-pointer but got a pointer, dereference
	if selectorType.Kind() != reflect.Ptr && valueType.Kind() == reflect.Ptr {
		if !valueType.Elem().AssignableTo(selectorType) {
			return nil, fmt.Errorf("incompatible types: selector expects %v but got %v", selectorType, valueType)
		}
		valueReflect := reflect.ValueOf(value)
		if valueReflect.IsNil() {
			// Handle nil pointer case by creating a zero value
			return reflect.Zero(selectorType).Interface(), nil
		}
		return valueReflect.Elem().Interface(), nil
	}

	// Handle slice conversions
	if selectorType.Kind() == reflect.Slice && valueType.Kind() == reflect.Slice {
		return c.adjustSliceValue(selectorType, value)
	}

	// For any other incompatible types
	return nil, fmt.Errorf("incompatible types: selector expects %v but got %v", selectorType, valueType)
}

// adjustSliceValue handles conversion between different slice types
func (c *BindingContext[T]) adjustSliceValue(selectorType reflect.Type, value interface{}) (interface{}, error) {
	valueSlice := reflect.ValueOf(value)
	length := valueSlice.Len()
	elemType := selectorType.Elem()

	// Create a new slice of the target type
	resultSlice := reflect.MakeSlice(selectorType, length, length)

	// Convert each element
	for i := 0; i < length; i++ {
		elem := valueSlice.Index(i).Interface()

		// Recursively adjust each element
		adjustedElem, err := c.adjustElementValue(elemType, elem)
		if err != nil {
			return nil, fmt.Errorf("error converting slice element at index %d: %w", i, err)
		}

		resultSlice.Index(i).Set(reflect.ValueOf(adjustedElem))
	}

	return resultSlice.Interface(), nil
}

// adjustElementValue adjusts a single element to match the target type
func (c *BindingContext[T]) adjustElementValue(targetType reflect.Type, value interface{}) (interface{}, error) {
	if value == nil {
		return reflect.Zero(targetType).Interface(), nil
	}

	valueType := reflect.TypeOf(value)
	valueReflect := reflect.ValueOf(value)

	// Direct assignment if compatible
	if valueType.AssignableTo(targetType) {
		return value, nil
	}

	// Handle pointer vs. non-pointer
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

	// Try basic numeric conversions
	if isNumericType(targetType) && isNumericType(valueType) {
		return convertNumeric(targetType, valueReflect)
	}

	// Handle string conversion if possible
	if targetType.Kind() == reflect.String {
		return fmt.Sprintf("%v", value), nil
	}

	return nil, fmt.Errorf("incompatible element types: target expects %v but got %v", targetType, valueType)
}

// isNumericType checks if a type is numeric (int, float, etc.)
func isNumericType(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return true
	}
	return false
}

// convertNumeric converts between numeric types
func convertNumeric(targetType reflect.Type, value reflect.Value) (interface{}, error) {
	var floatVal float64

	// Extract float value regardless of original type
	switch value.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		floatVal = float64(value.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		floatVal = float64(value.Uint())
	case reflect.Float32, reflect.Float64:
		floatVal = value.Float()
	default:
		return nil, fmt.Errorf("not a numeric type: %v", value.Type())
	}

	// Convert to target type
	switch targetType.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return int64(floatVal), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return uint64(floatVal), nil
	case reflect.Float32:
		return float32(floatVal), nil
	case reflect.Float64:
		return floatVal, nil
	default:
		return nil, fmt.Errorf("target is not a numeric type: %v", targetType)
	}
}
