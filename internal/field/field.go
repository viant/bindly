// Package field applies immutable field indexes compiled by x/shape.
package field

import (
	"fmt"
	"reflect"
)

type Access struct {
	Index []int
	Type  reflect.Type
}

func (a Access) Value(target reflect.Value) (reflect.Value, bool) {
	for _, index := range a.Index {
		for target.IsValid() && target.Kind() == reflect.Pointer {
			if target.IsNil() {
				return reflect.Value{}, false
			}
			target = target.Elem()
		}
		if !target.IsValid() || target.Kind() != reflect.Struct {
			return reflect.Value{}, false
		}
		target = target.Field(index)
	}
	return target, target.IsValid()
}
func (a Access) Set(target reflect.Value, value any) error {
	for _, index := range a.Index {
		for target.Kind() == reflect.Pointer {
			if target.IsNil() {
				if !target.CanSet() {
					return fmt.Errorf("field parent cannot be set")
				}
				target.Set(reflect.New(target.Type().Elem()))
			}
			target = target.Elem()
		}
		if target.Kind() != reflect.Struct {
			return fmt.Errorf("field parent is not a struct")
		}
		target = target.Field(index)
	}
	if !target.CanSet() {
		return fmt.Errorf("field cannot be set")
	}
	if value == nil {
		target.SetZero()
		return nil
	}
	actual := reflect.ValueOf(value)
	if !actual.Type().AssignableTo(target.Type()) {
		return fmt.Errorf("cannot assign %v to %v", actual.Type(), target.Type())
	}
	target.Set(actual)
	return nil
}
