package body

import (
	"encoding/json"
	"reflect"
	"strings"

	"github.com/viant/bindly/internal/field"
	xshape "github.com/viant/x/shape"
)

// markPresence records authored JSON keys on generated set-marker holders.
// JSON decoding remains encoding/json's responsibility; this traversal only
// projects source presence onto its already-decoded typed object graph.
func (s *Source) markPresence(target reflect.Value, raw json.RawMessage) error {
	for target.IsValid() && target.Kind() == reflect.Pointer {
		if target.IsNil() {
			return nil
		}
		target = target.Elem()
	}
	if !target.IsValid() {
		return nil
	}
	if target.Kind() == reflect.Slice || target.Kind() == reflect.Array {
		var values []json.RawMessage
		if err := json.Unmarshal(raw, &values); err != nil {
			return nil
		}
		for i, value := range values {
			if i >= target.Len() {
				break
			}
			if err := s.markPresence(target.Index(i), value); err != nil {
				return err
			}
		}
		return nil
	}
	if target.Kind() != reflect.Struct {
		return nil
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		return nil
	}
	fields, err := xshape.Linked(target.Type()).Fields()
	if err != nil {
		return err
	}
	marker := ""
	for _, item := range fields {
		if item.Tag.Get("setMarker") == "true" {
			marker = item.Name
			break
		}
	}
	if marker != "" {
		resolved, err := xshape.Linked(target.Type()).StructField(marker)
		if err != nil {
			return err
		}
		access := field.Access{Index: resolved.Index, Type: resolved.Type}
		// Presence is derived from business keys, never accepted from JSON. Reset
		// even an explicitly supplied marker before populating derived flags.
		value := reflect.Zero(resolved.Type)
		if resolved.Type.Kind() == reflect.Pointer {
			value = reflect.New(resolved.Type.Elem())
		}
		if err = access.Set(target, value.Interface()); err != nil {
			return err
		}
	}
	for _, item := range fields {
		if !item.Exported || item.Tag.Get("setMarker") == "true" {
			continue
		}
		name, _, _ := strings.Cut(item.Tag.Get("json"), ",")
		if name == "-" {
			continue
		}
		if name == "" {
			name = item.Name
		}
		value, present := object[name]
		if !present {
			for key, candidate := range object {
				if strings.EqualFold(key, name) {
					value, present = candidate, true
					break
				}
			}
		}
		if !present {
			continue
		}
		if marker != "" {
			if flag, err := xshape.Linked(target.Type()).StructField(marker + "." + item.Name); err == nil && flag.Type.Kind() == reflect.Bool {
				if err = (field.Access{Index: flag.Index, Type: flag.Type}).Set(target, true); err != nil {
					return err
				}
			}
		}
		child, ok := (field.Access{Index: item.Index, Type: item.ReflectedType}).Value(target)
		if ok {
			if err = s.markPresence(child, value); err != nil {
				return err
			}
		}
	}
	return nil
}
