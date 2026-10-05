package body

import (
	"encoding/json"
	"reflect"
	"strings"
)

func applyJSONPresence(raw []byte, value reflect.Value) {
	for value.IsValid() && value.Kind() == reflect.Ptr {
		if value.IsNil() {
			return
		}
		value = value.Elem()
	}
	if !value.IsValid() {
		return
	}
	switch value.Kind() {
	case reflect.Struct:
		fields := map[string]json.RawMessage{}
		if err := json.Unmarshal(raw, &fields); err != nil {
			return
		}
		marker := presenceMarker(value)
		for name, fieldRaw := range fields {
			fieldIndex, fieldName, ok := fieldByJSONName(value.Type(), name)
			if !ok {
				continue
			}
			if marker.IsValid() {
				flag := marker.FieldByName(fieldName)
				if flag.IsValid() && flag.CanSet() && flag.Kind() == reflect.Bool {
					flag.SetBool(true)
				}
			}
			applyJSONPresence(fieldRaw, value.Field(fieldIndex))
		}
	case reflect.Slice:
		items := []json.RawMessage{}
		if err := json.Unmarshal(raw, &items); err != nil {
			return
		}
		for index := 0; index < len(items) && index < value.Len(); index++ {
			applyJSONPresence(items[index], value.Index(index))
		}
	}
}

func presenceMarker(value reflect.Value) reflect.Value {
	field, ok := value.Type().FieldByName("Has")
	if !ok || field.Tag.Get("setMarker") != "true" {
		return reflect.Value{}
	}
	marker := value.FieldByIndex(field.Index)
	if !marker.CanSet() || marker.Kind() != reflect.Ptr || marker.Type().Elem().Kind() != reflect.Struct {
		return reflect.Value{}
	}
	if marker.IsNil() {
		marker.Set(reflect.New(marker.Type().Elem()))
	}
	return marker.Elem()
}

func fieldByJSONName(valueType reflect.Type, name string) (int, string, bool) {
	for index := 0; index < valueType.NumField(); index++ {
		field := valueType.Field(index)
		if field.Name == "Has" {
			continue
		}
		jsonName := strings.SplitN(field.Tag.Get("json"), ",", 2)[0]
		if field.Name == name || strings.EqualFold(field.Name, name) || jsonName != "-" && strings.EqualFold(jsonName, name) {
			return index, field.Name, true
		}
	}
	return -1, "", false
}
