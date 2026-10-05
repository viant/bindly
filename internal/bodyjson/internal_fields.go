package bodyjson

import (
	"bytes"
	"encoding"
	"encoding/json"
	"reflect"
	"strings"

	xshape "github.com/viant/x/shape"
)

type internalPlan struct {
	kind   reflect.Kind
	fields []internalField
	item   *internalPlan
}
type internalField struct {
	field  xshape.JSONField
	denied bool
	child  *internalPlan
}

func compileInternalPlan(target reflect.Type, memo map[reflect.Type]*internalPlan) (*internalPlan, error) {
	for target.Kind() == reflect.Pointer {
		if target.Implements(reflect.TypeFor[json.Unmarshaler]()) || target.Implements(reflect.TypeFor[encoding.TextUnmarshaler]()) {
			return nil, nil
		}
		target = target.Elem()
	}
	if target.Implements(reflect.TypeFor[json.Unmarshaler]()) || reflect.PointerTo(target).Implements(reflect.TypeFor[json.Unmarshaler]()) || target.Implements(reflect.TypeFor[encoding.TextUnmarshaler]()) || reflect.PointerTo(target).Implements(reflect.TypeFor[encoding.TextUnmarshaler]()) {
		return nil, nil
	}
	if plan, ok := memo[target]; ok {
		return plan, nil
	}
	plan := &internalPlan{kind: target.Kind()}
	memo[target] = plan
	switch target.Kind() {
	case reflect.Slice, reflect.Array, reflect.Map:
		child, err := compileInternalPlan(target.Elem(), memo)
		if err != nil {
			return nil, err
		}
		plan.item = child
	case reflect.Struct:
		fields, err := xshape.Linked(target).JSONFields()
		if err != nil {
			return nil, err
		}
		for _, field := range fields {
			item := internalField{field: field, denied: internalOwner(target, field.Field.Index)}
			if !item.denied {
				item.child, err = compileInternalPlan(field.Field.ReflectedType, memo)
				if err != nil {
					return nil, err
				}
			}
			plan.fields = append(plan.fields, item)
		}
	}
	return plan, nil
}
func internalOwner(root reflect.Type, index []int) bool {
	for _, i := range index {
		for root.Kind() == reflect.Pointer {
			root = root.Elem()
		}
		field := root.Field(i)
		if field.Tag.Get("internal") == "true" {
			return true
		}
		root = field.Type
	}
	return false
}
func (p *internalPlan) hasInternal(seen map[*internalPlan]bool) bool {
	if p == nil || seen[p] {
		return false
	}
	seen[p] = true
	if p.item.hasInternal(seen) {
		return true
	}
	for _, field := range p.fields {
		if field.denied || field.child.hasInternal(seen) {
			return true
		}
	}
	return false
}
func (p *internalPlan) field(name string) *internalField {
	for i := range p.fields {
		if p.fields[i].field.Name == name {
			return &p.fields[i]
		}
	}
	for i := range p.fields {
		if strings.EqualFold(p.fields[i].field.Name, name) {
			return &p.fields[i]
		}
	}
	return nil
}
func excludeInternal(raw []byte, target reflect.Type) ([]byte, error) {
	plan, err := compileInternalPlan(target, map[reflect.Type]*internalPlan{})
	if err != nil {
		return nil, err
	}
	if !plan.hasInternal(map[*internalPlan]bool{}) {
		return raw, nil
	}
	// Validate the complete document, including discarded values and suffixes,
	// before slicing it. RawMessage validation preserves native syntax errors.
	var valid json.RawMessage
	if err = json.Unmarshal(raw, &valid); err != nil {
		return nil, err
	}
	return plan.filter(raw)
}
func (p *internalPlan) filter(raw []byte) ([]byte, error) {
	if !p.hasInternal(map[*internalPlan]bool{}) || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return raw, nil
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return raw, nil
	}
	if (p.kind == reflect.Slice || p.kind == reflect.Array) && trimmed[0] == '[' {
		return p.filterArray(raw)
	}
	if (p.kind == reflect.Struct || p.kind == reflect.Map) && trimmed[0] == '{' {
		return p.filterObject(raw)
	}
	// A public type mismatch remains the ordinary decoder's error.
	return raw, nil
}
func (p *internalPlan) filterArray(raw []byte) ([]byte, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if _, err := decoder.Token(); err != nil {
		return nil, err
	}
	result := make([]byte, 0, len(raw))
	previous := 0
	changed := false
	for decoder.More() {
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, err
		}
		end := int(decoder.InputOffset())
		start := end - len(value)
		filtered, err := p.item.filter(value)
		if err != nil {
			return nil, err
		}
		changed = changed || !bytes.Equal(value, filtered)
		result = append(result, raw[previous:start]...)
		result = append(result, filtered...)
		previous = end
	}
	if _, err := decoder.Token(); err != nil {
		return nil, err
	}
	if !changed {
		return raw, nil
	}
	return append(result, raw[previous:]...), nil
}
func (p *internalPlan) filterObject(raw []byte) ([]byte, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if _, err := decoder.Token(); err != nil {
		return nil, err
	}
	opening := int(decoder.InputOffset())
	previous := opening
	changed := false
	var members [][]byte
	for decoder.More() {
		keyStart := previous
		for keyStart < len(raw) && (raw[keyStart] == ' ' || raw[keyStart] == '\t' || raw[keyStart] == '\r' || raw[keyStart] == '\n' || raw[keyStart] == ',') {
			keyStart++
		}
		keyToken, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		key := keyToken.(string)
		var value json.RawMessage
		if err = decoder.Decode(&value); err != nil {
			return nil, err
		}
		end := int(decoder.InputOffset())
		valueStart := end - len(value)
		previous = end
		child := p.item
		if p.kind == reflect.Struct {
			field := p.field(key)
			if field != nil {
				if field.denied {
					changed = true
					continue
				}
				child = field.child
			}
		}
		filtered, err := child.filter(value)
		if err != nil {
			return nil, err
		}
		changed = changed || !bytes.Equal(value, filtered)
		member := append([]byte(nil), raw[keyStart:valueStart]...)
		member = append(member, filtered...)
		members = append(members, member)
	}
	if _, err := decoder.Token(); err != nil {
		return nil, err
	}
	closing := int(decoder.InputOffset()) - 1
	if !changed {
		return raw, nil
	}
	result := append([]byte(nil), raw[:opening]...)
	for i, member := range members {
		if i > 0 {
			result = append(result, ',')
		}
		result = append(result, member...)
	}
	return append(result, raw[closing:]...), nil
}
