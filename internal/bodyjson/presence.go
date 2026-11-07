package bodyjson

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"

	"github.com/viant/bindly/internal/field"
	xshape "github.com/viant/x/shape"
)

// markPresence records authored JSON keys on generated set-marker holders.
// JSON decoding remains encoding/json's responsibility; this traversal only
// projects source presence onto its already-decoded typed object graph.
func MarkPresence(target reflect.Value, raw json.RawMessage, public bool, representations ...*xshape.Representation) error {
	var representation *xshape.Representation
	if target.IsValid() {
		representation = metadataForType(firstMetadata(representations), target.Type())
	}
	// Match filtering's ownership boundary before resetting markers or walking
	// children: custom decoders may derive sparse flags from aliases and nulls.
	if target.IsValid() && ownsJSONRepresentation(target.Type()) {
		return nil
	}
	for target.IsValid() && target.Kind() == reflect.Pointer {
		if target.IsNil() {
			return nil
		}
		target = target.Elem()
	}
	if !target.IsValid() {
		return nil
	}
	representation = metadataForType(representation, target.Type())
	if target.Kind() == reflect.Slice || target.Kind() == reflect.Array {
		var values []json.RawMessage
		if err := json.Unmarshal(raw, &values); err != nil {
			return nil
		}
		for i, value := range values {
			if i >= target.Len() {
				break
			}
			if err := MarkPresence(target.Index(i), value, public, metadataElement(representation)); err != nil {
				return err
			}
		}
		return nil
	}
	if target.Kind() == reflect.Map && representation != nil {
		return markMapMetadata(target, raw, public, representation)
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
	if public {
		plan, err := compileInternalPlan(target.Type(), nil, representation)
		if err != nil {
			return err
		}
		if plan != nil && plan.aliases && plan.kind == reflect.Struct {
			// Resolve synonyms in source order, using the same selected-field
			// identity as filtering and decoding, never map iteration order.
			canonical := map[string]json.RawMessage{}
			decoder := json.NewDecoder(bytes.NewReader(raw))
			if _, err := decoder.Token(); err != nil {
				return err
			}
			for decoder.More() {
				key, err := decoder.Token()
				if err != nil {
					return err
				}
				var member json.RawMessage
				if err = decoder.Decode(&member); err != nil {
					return err
				}
				if selected := plan.field(key.(string)); selected != nil && !selected.denied {
					canonical[selected.field.Name] = member
				}
			}
			return markCanonicalPresence(target, canonical, marker, plan)
		}
		if plan != nil && plan.hasInternal(map[*internalPlan]bool{}) && plan.kind == reflect.Struct {
			return markCanonicalPresence(target, object, marker, plan)
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
			if err = MarkPresence(child, value, public); err != nil {
				return err
			}
		}
	}
	return nil
}

func markCanonicalPresence(target reflect.Value, object map[string]json.RawMessage, marker string, plan *internalPlan) error {
	// Visit each selected field once. Native presence prefers its exact JSON
	// name even when decoding merged values from other case variants.
	for i := range plan.fields {
		item := &plan.fields[i]
		if item.denied || item.field.Field.Tag.Get("setMarker") == "true" {
			continue
		}
		raw, present := object[item.field.Name]
		if !present {
			for key, candidate := range object {
				if strings.EqualFold(key, item.field.Name) && plan.field(key) == item {
					raw, present = candidate, true
					break
				}
			}
		}
		if !present {
			continue
		}
		if marker != "" {
			canonical := target.Type().FieldByIndex(item.field.Field.Index)
			if flag, err := xshape.Linked(target.Type()).StructField(marker + "." + canonical.Name); err == nil && flag.Type.Kind() == reflect.Bool {
				if err = (field.Access{Index: flag.Index, Type: flag.Type}).Set(target, true); err != nil {
					return err
				}
			}
		}
		child, ok := (field.Access{Index: item.field.Field.Index, Type: item.field.Field.ReflectedType}).Value(target)
		if ok {
			if err := MarkPresence(child, raw, true, item.representation); err != nil {
				return err
			}
		}
	}
	return nil
}

func metadataElement(representation *xshape.Representation) *xshape.Representation {
	if representation == nil {
		return nil
	}
	return representation.Elem()
}
func markMapMetadata(target reflect.Value, raw json.RawMessage, public bool, metadata *xshape.Representation) error {
	if target.IsNil() {
		return nil
	}
	// Keep source key order when distinct JSON spellings address one integer key.
	decoder := json.NewDecoder(bytes.NewReader(raw))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return nil
	}
	type entry struct {
		key reflect.Value
		raw json.RawMessage
	}
	entries := map[any]entry{}
	for decoder.More() {
		name, err := decoder.Token()
		if err != nil {
			return err
		}
		var member json.RawMessage
		if err = decoder.Decode(&member); err != nil {
			return err
		}
		encoded, _ := json.Marshal(name)
		object := append([]byte{'{'}, encoded...)
		object = append(object, []byte(":null}")...)
		keys := reflect.New(reflect.MapOf(target.Type().Key(), reflect.TypeFor[json.RawMessage]()))
		if err = json.Unmarshal(object, keys.Interface()); err != nil {
			return err
		}
		key := keys.Elem().MapKeys()[0]
		entries[key.Interface()] = entry{key, member}
	}
	for _, item := range entries {
		value := target.MapIndex(item.key)
		if !value.IsValid() {
			continue
		}
		copy := reflect.New(value.Type()).Elem()
		copy.Set(value)
		if err = MarkPresence(copy, item.raw, public, metadataElement(metadata)); err != nil {
			return err
		}
		target.SetMapIndex(item.key, copy)
	}
	return nil
}
