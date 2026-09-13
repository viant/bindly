package conv

import (
	"encoding"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"time"
)

// ValueConverter converts wire or authored values to the planned source type.
// Codecs run after this conversion and own destination-specific semantics.
type ValueConverter struct {
	TimeLayout string
	// DisallowJSON disables implicit structured decoding for non-JSON sources.
	DisallowJSON bool
}

func (c ValueConverter) Convert(value any, target reflect.Type) (any, error) {
	if target == nil || value == nil {
		return value, nil
	}
	actual := reflect.ValueOf(value)
	if actual.Type().AssignableTo(target) {
		return value, nil
	}
	if target.Kind() == reflect.Pointer {
		converted, err := c.Convert(value, target.Elem())
		if err != nil {
			return nil, err
		}
		result := reflect.New(target.Elem())
		if converted != nil {
			result.Elem().Set(reflect.ValueOf(converted))
		}
		return result.Interface(), nil
	}
	if actual.Kind() == reflect.Pointer {
		if actual.IsNil() {
			return nil, nil
		}
		return c.Convert(actual.Elem().Interface(), target)
	}
	if strings, ok := value.([]string); ok && target.Kind() != reflect.Slice && target.Kind() != reflect.Array {
		if len(strings) != 1 {
			return nil, fmt.Errorf("expected one value for %v, got %d", target, len(strings))
		}
		return c.Convert(strings[0], target)
	}
	if target == reflect.TypeOf(json.RawMessage{}) {
		switch raw := value.(type) {
		case string:
			return json.RawMessage(raw), nil
		case []byte:
			return json.RawMessage(append([]byte(nil), raw...)), nil
		}
	}
	if target == reflect.TypeOf(time.Time{}) {
		layout := c.TimeLayout
		if layout == "" {
			layout = time.RFC3339
		}
		return time.Parse(layout, fmt.Sprint(value))
	}
	if target.Kind() == reflect.Slice || target.Kind() == reflect.Array {
		if target == reflect.TypeOf([]byte{}) && actual.Kind() == reflect.String {
			return base64.StdEncoding.DecodeString(actual.String())
		}
		if actual.Kind() == reflect.String {
			return c.decodeJSON([]byte(actual.String()), target)
		}
		if actual.Kind() != reflect.Slice && actual.Kind() != reflect.Array {
			return nil, fmt.Errorf("expected collection for %v, got %T", target, value)
		}
		if target.Kind() == reflect.Array && target.Len() != actual.Len() {
			return nil, fmt.Errorf("array length mismatch for %v", target)
		}
		var result reflect.Value
		if target.Kind() == reflect.Slice {
			result = reflect.MakeSlice(target, actual.Len(), actual.Len())
		} else {
			result = reflect.New(target).Elem()
		}
		for i := 0; i < actual.Len(); i++ {
			item, err := c.Convert(actual.Index(i).Interface(), target.Elem())
			if err != nil {
				return nil, err
			}
			if item != nil {
				result.Index(i).Set(reflect.ValueOf(item))
			}
		}
		return result.Interface(), nil
	}
	result := reflect.New(target)
	if text, ok := result.Interface().(encoding.TextUnmarshaler); ok {
		if err := text.UnmarshalText([]byte(fmt.Sprint(value))); err != nil {
			return nil, err
		}
		return result.Elem().Interface(), nil
	}
	raw := fmt.Sprint(value)
	switch target.Kind() {
	case reflect.String:
		result.Elem().SetString(raw)
	case reflect.Bool:
		v, err := strconv.ParseBool(raw)
		if err != nil {
			return nil, err
		}
		result.Elem().SetBool(v)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v, err := strconv.ParseInt(raw, 10, target.Bits())
		if err != nil {
			return nil, err
		}
		result.Elem().SetInt(v)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		v, err := strconv.ParseUint(raw, 10, target.Bits())
		if err != nil {
			return nil, err
		}
		result.Elem().SetUint(v)
	case reflect.Float32, reflect.Float64:
		v, err := strconv.ParseFloat(raw, target.Bits())
		if err != nil {
			return nil, err
		}
		result.Elem().SetFloat(v)
	case reflect.Struct, reflect.Map:
		if actual.Kind() == reflect.String {
			return c.decodeJSON([]byte(raw), target)
		}
		encoded, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		return c.decodeJSON(encoded, target)
	default:
		return nil, fmt.Errorf("cannot convert %T to %v", value, target)
	}
	return result.Elem().Interface(), nil
}

func (c ValueConverter) decodeJSON(raw []byte, target reflect.Type) (any, error) {
	if c.DisallowJSON {
		return nil, fmt.Errorf("JSON conversion to %v is not allowed for this source", target)
	}
	result := reflect.New(target)
	if err := json.Unmarshal(raw, result.Interface()); err != nil {
		return nil, err
	}
	return result.Elem().Interface(), nil
}
