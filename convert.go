package bindly

import (
	"fmt"
	"reflect"
	"strconv"
)

func convertValue(targetType reflect.Type, input interface{}) (interface{}, error) {
	if input == nil {
		return nil, nil
	}
	return convertReflectValue(targetType, reflect.ValueOf(input))
}

func convertReflectValue(targetType reflect.Type, source reflect.Value) (interface{}, error) {
	if source.Kind() == reflect.Interface && !source.IsNil() {
		source = source.Elem()
	}
	if source.Type().AssignableTo(targetType) {
		return source.Interface(), nil
	}
	if targetType.Kind() == reflect.Interface && source.Type().Implements(targetType) {
		return source.Interface(), nil
	}
	if source.Kind() == reflect.Ptr && targetType.Kind() != reflect.Ptr {
		if source.IsNil() {
			return reflect.Zero(targetType).Interface(), nil
		}
		return convertReflectValue(targetType, source.Elem())
	}
	if targetType.Kind() == reflect.Ptr {
		if source.Kind() == reflect.Ptr && source.IsNil() {
			return reflect.Zero(targetType).Interface(), nil
		}
		if source.Kind() == reflect.Ptr {
			source = source.Elem()
		}
		converted, err := convertReflectValue(targetType.Elem(), source)
		if err != nil {
			return nil, err
		}
		result := reflect.New(targetType.Elem())
		result.Elem().Set(reflect.ValueOf(converted))
		return result.Interface(), nil
	}
	if targetType.Kind() == reflect.Interface && reflect.PointerTo(source.Type()).Implements(targetType) {
		pointer := reflect.New(source.Type())
		pointer.Elem().Set(source)
		return pointer.Interface(), nil
	}
	if targetType.Kind() == reflect.Slice {
		return convertSlice(targetType, source)
	}
	if targetType.Kind() == reflect.Array {
		return convertArray(targetType, source)
	}
	if source.Kind() == reflect.String {
		return convertString(targetType, source.String())
	}
	if isNumericKind(targetType.Kind()) && isNumericKind(source.Kind()) {
		return convertNumericValue(targetType, source)
	}
	if source.Type().ConvertibleTo(targetType) {
		return source.Convert(targetType).Interface(), nil
	}
	return nil, fmt.Errorf("incompatible types: target expects %v but got %v", targetType, source.Type())
}

func convertArray(targetType reflect.Type, source reflect.Value) (interface{}, error) {
	if source.Kind() != reflect.Slice && source.Kind() != reflect.Array {
		single := reflect.New(targetType).Elem()
		converted, err := convertReflectValue(targetType.Elem(), source)
		if err != nil {
			return nil, err
		}
		single.Index(0).Set(reflect.ValueOf(converted))
		return single.Interface(), nil
	}
	if source.Len() > targetType.Len() {
		return nil, fmt.Errorf("cannot bind %d values to %v", source.Len(), targetType)
	}
	result := reflect.New(targetType).Elem()
	for i := 0; i < source.Len(); i++ {
		converted, err := convertReflectValue(targetType.Elem(), source.Index(i))
		if err != nil {
			return nil, fmt.Errorf("convert array element %d: %w", i, err)
		}
		result.Index(i).Set(reflect.ValueOf(converted))
	}
	return result.Interface(), nil
}

func convertSlice(targetType reflect.Type, source reflect.Value) (interface{}, error) {
	if source.Kind() != reflect.Slice && source.Kind() != reflect.Array {
		converted, err := convertReflectValue(targetType.Elem(), source)
		if err != nil {
			return nil, err
		}
		result := reflect.MakeSlice(targetType, 1, 1)
		result.Index(0).Set(reflect.ValueOf(converted))
		return result.Interface(), nil
	}
	result := reflect.MakeSlice(targetType, source.Len(), source.Len())
	for i := 0; i < source.Len(); i++ {
		converted, err := convertReflectValue(targetType.Elem(), source.Index(i))
		if err != nil {
			return nil, fmt.Errorf("convert slice element %d: %w", i, err)
		}
		result.Index(i).Set(reflect.ValueOf(converted))
	}
	return result.Interface(), nil
}

func convertString(targetType reflect.Type, source string) (interface{}, error) {
	result := reflect.New(targetType).Elem()
	switch targetType.Kind() {
	case reflect.String:
		result.SetString(source)
	case reflect.Bool:
		value, err := strconv.ParseBool(source)
		if err != nil {
			return nil, err
		}
		result.SetBool(value)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		value, err := strconv.ParseInt(source, 10, targetType.Bits())
		if err != nil {
			return nil, err
		}
		result.SetInt(value)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		value, err := strconv.ParseUint(source, 10, targetType.Bits())
		if err != nil {
			return nil, err
		}
		result.SetUint(value)
	case reflect.Float32, reflect.Float64:
		value, err := strconv.ParseFloat(source, targetType.Bits())
		if err != nil {
			return nil, err
		}
		result.SetFloat(value)
	default:
		return nil, fmt.Errorf("cannot convert string to %v", targetType)
	}
	return result.Interface(), nil
}

func convertNumericValue(targetType reflect.Type, source reflect.Value) (interface{}, error) {
	result := reflect.New(targetType).Elem()
	switch targetType.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		value, err := numericInt64(source)
		if err != nil || result.OverflowInt(value) {
			if err == nil {
				err = fmt.Errorf("value %v overflows %v", source.Interface(), targetType)
			}
			return nil, err
		}
		result.SetInt(value)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		value, err := numericUint64(source)
		if err != nil || result.OverflowUint(value) {
			if err == nil {
				err = fmt.Errorf("value %v overflows %v", source.Interface(), targetType)
			}
			return nil, err
		}
		result.SetUint(value)
	case reflect.Float32, reflect.Float64:
		value, err := numericFloat64(source)
		if err != nil || result.OverflowFloat(value) {
			if err == nil {
				err = fmt.Errorf("value %v overflows %v", source.Interface(), targetType)
			}
			return nil, err
		}
		result.SetFloat(value)
	default:
		return nil, fmt.Errorf("target is not numeric: %v", targetType)
	}
	return result.Interface(), nil
}

func numericInt64(value reflect.Value) (int64, error) {
	switch value.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return value.Int(), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		unsigned := value.Uint()
		if unsigned > uint64(^uint64(0)>>1) {
			return 0, fmt.Errorf("value %d overflows int64", unsigned)
		}
		return int64(unsigned), nil
	case reflect.Float32, reflect.Float64:
		float := value.Float()
		integer := int64(float)
		if float != float64(integer) {
			return 0, fmt.Errorf("value %v is not an integer", float)
		}
		return integer, nil
	default:
		return 0, fmt.Errorf("value is not numeric: %v", value.Type())
	}
}

func numericUint64(value reflect.Value) (uint64, error) {
	switch value.Kind() {
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return value.Uint(), nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		integer := value.Int()
		if integer < 0 {
			return 0, fmt.Errorf("negative value %d cannot convert to uint", integer)
		}
		return uint64(integer), nil
	case reflect.Float32, reflect.Float64:
		float := value.Float()
		if float < 0 || float != float64(uint64(float)) {
			return 0, fmt.Errorf("value %v is not an unsigned integer", float)
		}
		return uint64(float), nil
	default:
		return 0, fmt.Errorf("value is not numeric: %v", value.Type())
	}
}

func numericFloat64(value reflect.Value) (float64, error) {
	switch value.Kind() {
	case reflect.Float32, reflect.Float64:
		return value.Float(), nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return float64(value.Int()), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return float64(value.Uint()), nil
	default:
		return 0, fmt.Errorf("value is not numeric: %v", value.Type())
	}
}

func isNumericKind(kind reflect.Kind) bool {
	switch kind {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return true
	default:
		return false
	}
}
