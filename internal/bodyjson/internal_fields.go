package bodyjson

import (
	"bytes"
	"encoding"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/viant/tagly/format"

	xshape "github.com/viant/x/shape"
)

type internalPlan struct {
	representation *xshape.Representation
	kind           reflect.Kind
	fields         []internalField
	item           *internalPlan
	pointer        bool
	arrayLen       int
	timeLeaf       bool
	pointerTime    bool
	internal       bool
	inputs         bool
	dates          bool
	dateErr        error
}
type internalField struct {
	representation *xshape.Representation
	field          xshape.JSONField
	denied         bool
	child          *internalPlan
	input          inputRule
}

type internalPlanResult struct {
	plan *internalPlan
	err  error
}

// Plans are built privately, including recursive relevance, before publication.
// Only immutable, type-owned metadata is shared; raw bodies remain call-local.
var internalPlans sync.Map

type internalPlanKey struct {
	target         reflect.Type
	representation *xshape.Representation
}

func compileInternalPlan(target reflect.Type, _ map[reflect.Type]*internalPlan, representations ...*xshape.Representation) (*internalPlan, error) {
	representation := metadataForType(firstMetadata(representations), target)
	if representation != nil && representation.Original() != target {
		return nil, fmt.Errorf("body metadata type mismatch")
	}
	if cached, ok := internalPlans.Load(target); ok && representation == nil {
		result := cached.(internalPlanResult)
		return result.plan, result.err
	}
	memo := map[internalPlanKey]*internalPlan{}
	plan, err := buildInternalPlan(target, memo, representation)
	if err == nil {
		// A fixed point propagates relevance through mutually recursive types once
		// at compilation, rather than rediscovering it for every JSON value.
		changed := true
		for changed {
			changed = false
			for _, node := range memo {
				internal, dates, inputs := node.internal, node.dates, node.inputs
				dateErr := node.dateErr
				if node.item != nil {
					internal, dates = internal || node.item.internal, dates || node.item.dates
					inputs = inputs || node.item.inputs
					if dateErr == nil {
						dateErr = node.item.dateErr
					}
				}
				for _, field := range node.fields {
					internal = internal || field.denied
					inputs = inputs || field.input.active()
					if field.child != nil {
						internal, dates = internal || field.child.internal, dates || field.child.dates
						inputs = inputs || field.child.inputs
						if dateErr == nil {
							dateErr = field.child.dateErr
						}
					}
				}
				if internal != node.internal || dates != node.dates || inputs != node.inputs || node.dateErr == nil && dateErr != nil {
					node.internal, node.dates, node.inputs, node.dateErr, changed = internal, dates, inputs, dateErr, true
				}
			}
		}
	}
	// Caller metadata follows its owning variant's lifetime; do not retain
	// arbitrary per-request variants in the global linked-type cache.
	if representation != nil {
		return plan, err
	}
	published, _ := internalPlans.LoadOrStore(target, internalPlanResult{plan: plan, err: err})
	result := published.(internalPlanResult)
	return result.plan, result.err
}

func buildInternalPlan(target reflect.Type, memo map[internalPlanKey]*internalPlan, representation *xshape.Representation) (*internalPlan, error) {
	representation = metadataForType(representation, target)
	key := internalPlanKey{target, representation}
	if target == nil {
		return nil, nil
	}
	if plan, ok := memo[key]; ok {
		return plan, nil
	}
	original := target
	for target.Kind() == reflect.Pointer {
		target = target.Elem()
	}
	representation = metadataForType(representation, target)
	// The builtin time owner is the one deliberate exception. Distinct named
	// time types and all other JSON/Text owners keep their own representation.
	if target == reflect.TypeFor[time.Time]() {
		plan := &internalPlan{timeLeaf: true, pointerTime: original.Kind() == reflect.Pointer, dates: true}
		memo[key] = plan
		return plan, nil
	}
	if ownsJSONRepresentation(original) {
		return nil, nil
	}
	plan := &internalPlan{kind: target.Kind(), pointer: original.Kind() == reflect.Pointer, representation: representation, internal: representation != nil}
	if target.Kind() == reflect.Array {
		plan.arrayLen = target.Len()
	}
	memo[key] = plan
	switch target.Kind() {
	case reflect.Slice, reflect.Array, reflect.Map:
		var childShape *xshape.Representation
		if representation != nil {
			childShape = representation.Elem()
		}
		child, err := buildInternalPlan(target.Elem(), memo, childShape)
		if err != nil {
			return nil, err
		}
		plan.item = child
	case reflect.Struct:
		var fields []xshape.JSONField
		var err error
		if representation != nil {
			fields, err = representation.JSONFields()
		} else {
			fields, err = xshape.Linked(target).JSONFields()
		}
		if err != nil {
			return nil, err
		}
		for _, field := range fields {
			var childShape *xshape.Representation
			if representation != nil {
				childShape = representation.FieldShapeByIndex(field.Field.Index)
			}
			item := internalField{field: field, representation: childShape, denied: internalOwnerMetadata(target, field.Field.Index, representation)}
			if !item.denied {
				item.input, err = compileInputRule(field.Field.Tag, field.Field.ReflectedType)
				if err != nil {
					return nil, err
				}
				item.child, err = buildInternalPlan(field.Field.ReflectedType, memo, childShape)
				if err != nil {
					return nil, err
				}
				// Explicit field-layout policy belongs to its existing owner. In
				// particular, a formatted zero pointer must not acquire default nilness.
				if item.child != nil && item.child.timeLeaf {
					formatting, err := format.Parse(field.Field.Tag)
					if err != nil {
						// Date metadata belongs to native date decoding, not the
						// registered decoder's filtering/presence contract.
						if plan.dateErr == nil {
							plan.dateErr = err
						}
					}
					if formatting != nil && formatting.TimeLayout != "" {
						item.child = nil
					}
				}
			}
			plan.fields = append(plan.fields, item)
		}
		if representation != nil {
			// Keep original JSON spellings that disappeared from the effective
			// contract as denied entries. JSON winner selection omits json:"-"
			// fields, but configured decoders must not receive their raw values.
			originalFields, err := xshape.Linked(target).JSONFields()
			if err != nil {
				return nil, err
			}
			for _, originalField := range originalFields {
				retained := false
				for _, selected := range plan.fields {
					if strings.EqualFold(selected.field.Name, originalField.Name) {
						retained = true
						break
					}
				}
				if !retained {
					plan.fields = append(plan.fields, internalField{field: originalField, denied: true})
				}
			}
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
func (p *internalPlan) hasInternal(_ map[*internalPlan]bool) bool {
	return p != nil && p.internal
}
func (p *internalPlan) relevant(normalizeDates bool) bool {
	return p != nil && (p.internal || normalizeDates && (p.dates || p.inputs))
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
	return filterPublic(raw, target, false)
}
func filterPublic(raw []byte, target reflect.Type, normalizeDates bool, representations ...*xshape.Representation) ([]byte, error) {
	plan, err := compileInternalPlan(target, nil, representations...)
	if err != nil {
		return nil, err
	}
	if normalizeDates && plan != nil && plan.dateErr != nil {
		return nil, plan.dateErr
	}
	if !plan.relevant(normalizeDates) {
		return raw, nil
	}
	// Validate the complete document, including discarded values and suffixes,
	// before slicing it. RawMessage validation preserves native syntax errors.
	var valid json.RawMessage
	if err = json.Unmarshal(raw, &valid); err != nil {
		return nil, err
	}
	return plan.filter(raw, normalizeDates, &dateState{values: map[string][]byte{}}, "$")
}
func (p *internalPlan) filter(raw []byte, normalizeDates bool, dates *dateState, path string) ([]byte, error) {
	if !p.relevant(normalizeDates) {
		return raw, nil
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		if normalizeDates && p.timeLeaf {
			if p.pointerTime {
				if previous := dates.values[path]; previous != nil {
					return previous, nil
				}
				return raw, nil
			}
			return normalizeLegacyTime(raw, false, dates, path)
		}
		if p.pointerTime || p.pointer || p.kind == reflect.Map || p.kind == reflect.Slice {
			dates.clear(path)
		}
		return raw, nil
	}
	if normalizeDates && p.timeLeaf {
		return normalizeLegacyTime(raw, p.pointerTime, dates, path)
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return raw, nil
	}
	if (p.kind == reflect.Slice || p.kind == reflect.Array) && trimmed[0] == '[' {
		return p.filterArray(raw, normalizeDates, dates, path)
	}
	if (p.kind == reflect.Struct || p.kind == reflect.Map) && trimmed[0] == '{' {
		return p.filterObject(raw, normalizeDates, dates, path)
	}
	// A public type mismatch remains the ordinary decoder's error.
	return raw, nil
}
func (p *internalPlan) filterArray(raw []byte, normalizeDates bool, dates *dateState, path string) ([]byte, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if _, err := decoder.Token(); err != nil {
		return nil, err
	}
	result := make([]byte, 0, len(raw))
	previous := 0
	changed := false
	index := 0
	for decoder.More() {
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, err
		}
		end := int(decoder.InputOffset())
		start := end - len(value)
		normalizeItem := normalizeDates && (p.kind != reflect.Array || index < p.arrayLen)
		filtered, err := p.item.filter(value, normalizeItem, dates, path+"/i"+strconv.Itoa(index))
		if err != nil {
			return nil, err
		}
		changed = changed || !bytes.Equal(value, filtered)
		result = append(result, raw[previous:start]...)
		result = append(result, filtered...)
		previous = end
		index++
	}
	if p.kind == reflect.Array {
		for i := index; i < p.arrayLen; i++ {
			dates.clear(path + "/i" + strconv.Itoa(i))
		}
	} else if p.kind == reflect.Slice && index == 0 {
		// Native decoding replaces an empty slice with a fresh empty slice.
		dates.clear(path)
	}
	if _, err := decoder.Token(); err != nil {
		return nil, err
	}
	if !changed {
		return raw, nil
	}
	return append(result, raw[previous:]...), nil
}
func (p *internalPlan) filterObject(raw []byte, normalizeDates bool, dates *dateState, path string) ([]byte, error) {
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
		childPath := path + "/m" + strconv.Itoa(len(key)) + ":" + key
		if p.kind == reflect.Map {
			dates.clear(childPath)
		}
		if p.kind == reflect.Struct {
			field := p.field(key)
			if field != nil {
				if field.denied {
					changed = true
					continue
				}
				if normalizeDates && field.input.active() {
					converted, omit := field.input.apply(value)
					if omit {
						changed = true
						continue
					}
					changed = changed || !bytes.Equal(value, converted)
					value = converted
				}
				child = field.child
				childPath = path + "/f" + strconv.Itoa(len(field.field.Name)) + ":" + field.field.Name
			}
		}
		filtered, err := child.filter(value, normalizeDates, dates, childPath)
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

// ownsJSONRepresentation is shared by filtering and presence reconstruction:
// an authored decoder owns the entire representation of its decoded subtree.
func ownsJSONRepresentation(target reflect.Type) bool {
	if target == nil {
		return false
	}
	for {
		if target.Implements(reflect.TypeFor[json.Unmarshaler]()) || target.Implements(reflect.TypeFor[encoding.TextUnmarshaler]()) {
			return true
		}
		if target.Kind() != reflect.Pointer {
			break
		}
		target = target.Elem()
	}
	return reflect.PointerTo(target).Implements(reflect.TypeFor[json.Unmarshaler]()) || reflect.PointerTo(target).Implements(reflect.TypeFor[encoding.TextUnmarshaler]())
}

func firstMetadata(representations []*xshape.Representation) *xshape.Representation {
	if len(representations) == 0 {
		return nil
	}
	return representations[0]
}
func metadataForType(representation *xshape.Representation, target reflect.Type) *xshape.Representation {
	if representation == nil || !representation.Changed() {
		return nil
	}
	for representation.Original() != target && representation.Original().Kind() == reflect.Pointer {
		representation = representation.Elem()
	}
	return representation
}
func internalOwnerMetadata(target reflect.Type, index []int, representation *xshape.Representation) bool {
	if representation == nil {
		return internalOwner(target, index)
	}
	for _, i := range index {
		for representation.Original().Kind() == reflect.Pointer {
			representation = representation.Elem()
		}
		if representation.Field(i).Tag.Get("internal") == "true" {
			return true
		}
		representation = representation.FieldShape(i)
	}
	return false
}
