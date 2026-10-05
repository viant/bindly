package bindly

import (
	"fmt"
	"reflect"
)

func (b *Binding) setReflectValue(target reflect.Value, value interface{}) error {
	if !target.IsValid() || target.Kind() != reflect.Ptr || target.IsNil() {
		return fmt.Errorf("binding target must be a non-nil pointer")
	}
	field, err := fieldByIndex(target.Elem(), b.index)
	if err != nil {
		return fmt.Errorf("set binding destination %q: %w", b.destinationPath(), err)
	}
	if !field.CanSet() {
		return fmt.Errorf("binding destination %q cannot be set", b.destinationPath())
	}
	if value == nil {
		field.Set(reflect.Zero(field.Type()))
		markBoundField(target.Elem(), b.index)
		return nil
	}
	actual := reflect.ValueOf(value)
	if !actual.Type().AssignableTo(field.Type()) {
		return fmt.Errorf("binding value type %s is not assignable to %s", actual.Type(), field.Type())
	}
	field.Set(actual)
	markBoundField(target.Elem(), b.index)
	return nil
}

func markBoundField(root reflect.Value, index []int) {
	if len(index) == 0 {
		return
	}
	current := root
	for _, fieldIndex := range index[:len(index)-1] {
		for current.Kind() == reflect.Ptr {
			if current.IsNil() {
				return
			}
			current = current.Elem()
		}
		if current.Kind() != reflect.Struct || fieldIndex >= current.NumField() {
			return
		}
		current = current.Field(fieldIndex)
	}
	for current.Kind() == reflect.Ptr {
		if current.IsNil() {
			return
		}
		current = current.Elem()
	}
	if current.Kind() != reflect.Struct {
		return
	}
	fieldIndex := index[len(index)-1]
	if fieldIndex >= current.NumField() {
		return
	}
	fieldName := current.Type().Field(fieldIndex).Name
	markerField, ok := current.Type().FieldByName("Has")
	if !ok || markerField.Tag.Get("setMarker") != "true" {
		return
	}
	marker := current.FieldByIndex(markerField.Index)
	if !marker.CanSet() || marker.Kind() != reflect.Ptr || marker.Type().Elem().Kind() != reflect.Struct {
		return
	}
	if marker.IsNil() {
		marker.Set(reflect.New(marker.Type().Elem()))
	}
	flag := marker.Elem().FieldByName(fieldName)
	if flag.IsValid() && flag.CanSet() && flag.Kind() == reflect.Bool {
		flag.SetBool(true)
	}
}

func fieldByIndex(value reflect.Value, index []int) (reflect.Value, error) {
	current := value
	for position, fieldIndex := range index {
		for current.Kind() == reflect.Ptr {
			if current.IsNil() {
				if !current.CanSet() {
					return reflect.Value{}, fmt.Errorf("nil pointer at index %d cannot be initialized", position)
				}
				current.Set(reflect.New(current.Type().Elem()))
			}
			current = current.Elem()
		}
		if current.Kind() != reflect.Struct || fieldIndex >= current.NumField() {
			return reflect.Value{}, fmt.Errorf("invalid field index at position %d", position)
		}
		current = current.Field(fieldIndex)
	}
	return current, nil
}
