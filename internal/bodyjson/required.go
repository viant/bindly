package bodyjson

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
)

// ValidateRequired checks explicit required/optional field metadata using the
// same canonical JSON field selection as public decoding. It is opt-in, and
// checks presence, not truthiness or membership. Supplied null counts as present;
// the decoder and the field's value contract retain null/type semantics.
func ValidateRequired(raw []byte, target reflect.Type) error {
	plan, err := compileInternalPlan(target, nil)
	if err != nil {
		return err
	}
	return plan.validateRequired(raw, "$")
}

func (p *internalPlan) validateRequired(raw []byte, path string) error {
	if p == nil || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil
	}
	switch p.kind {
	case reflect.Slice, reflect.Array:
		var items []json.RawMessage
		if err := json.Unmarshal(raw, &items); err != nil {
			return err
		}
		for i, item := range items {
			if p.kind == reflect.Array && i >= p.arrayLen {
				break
			}
			if err := p.item.validateRequired(item, fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
	case reflect.Map:
		var items map[string]json.RawMessage
		if err := json.Unmarshal(raw, &items); err != nil {
			return err
		}
		for key, item := range items {
			if err := p.item.validateRequired(item, path+"["+strconv.Quote(key)+"]"); err != nil {
				return err
			}
		}
	case reflect.Struct:
		var values map[string]json.RawMessage
		if err := json.Unmarshal(raw, &values); err != nil {
			return err
		}
		for i := range p.fields {
			field := &p.fields[i]
			if field.denied || field.field.Field.Tag.Get("setMarker") == "true" {
				continue
			}
			value, present := values[field.field.Name]
			if !present {
				for key, candidate := range values {
					if p.field(key) == field {
						value, present = candidate, true
						break
					}
				}
			}
			required := field.field.Field.Tag.Get("required") == "true"
			if field.field.Field.Tag.Get("optional") == "true" {
				required = false
			}
			if required && !present {
				return fmt.Errorf("field %s.%s is required", path, field.field.Name)
			}
			if present {
				if err := field.child.validateRequired(value, path+"."+field.field.Name); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
