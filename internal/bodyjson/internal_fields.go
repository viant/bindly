package bodyjson

import (
	"bytes"
	"encoding"
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/viant/tagly/format"

	xshape "github.com/viant/x/shape"
)

type internalPlan struct {
	kind        reflect.Kind
	fields      []internalField
	item        *internalPlan
	pointer     bool
	arrayLen    int
	timeLeaf    bool
	pointerTime bool
	internal    bool
	dates       bool
	dateErr     error
}
type internalField struct {
	field  xshape.JSONField
	denied bool
	child  *internalPlan
}

type internalPlanResult struct {
	plan *internalPlan
	err  error
}

// Plans are built privately, including recursive relevance, before publication.
// Only immutable, type-owned metadata is shared; raw bodies remain call-local.
var internalPlans sync.Map

func compileInternalPlan(target reflect.Type, _ map[reflect.Type]*internalPlan) (*internalPlan, error) {
	if cached, ok := internalPlans.Load(target); ok {
		result := cached.(internalPlanResult)
		return result.plan, result.err
	}
	memo := map[reflect.Type]*internalPlan{}
	plan, err := buildInternalPlan(target, memo)
	if err == nil {
		// A fixed point propagates relevance through mutually recursive types once
		// at compilation, rather than rediscovering it for every JSON value.
		changed := true
		for changed {
			changed = false
			for _, node := range memo {
				internal, dates := node.internal, node.dates
				dateErr := node.dateErr
				if node.item != nil {
					internal, dates = internal || node.item.internal, dates || node.item.dates
					if dateErr == nil {
						dateErr = node.item.dateErr
					}
				}
				for _, field := range node.fields {
					internal = internal || field.denied
					if field.child != nil {
						internal, dates = internal || field.child.internal, dates || field.child.dates
						if dateErr == nil {
							dateErr = field.child.dateErr
						}
					}
				}
				if internal != node.internal || dates != node.dates || node.dateErr == nil && dateErr != nil {
					node.internal, node.dates, node.dateErr, changed = internal, dates, dateErr, true
				}
			}
		}
	}
	published, _ := internalPlans.LoadOrStore(target, internalPlanResult{plan: plan, err: err})
	result := published.(internalPlanResult)
	return result.plan, result.err
}

func buildInternalPlan(target reflect.Type, memo map[reflect.Type]*internalPlan) (*internalPlan, error) {
	if target == nil {
		return nil, nil
	}
	if plan, ok := memo[target]; ok {
		return plan, nil
	}
	original := target
	for target.Kind() == reflect.Pointer {
		target = target.Elem()
	}
	// The builtin time owner is the one deliberate exception. Distinct named
	// time types and all other JSON/Text owners keep their own representation.
	if target == reflect.TypeFor[time.Time]() {
		plan := &internalPlan{timeLeaf: true, pointerTime: original.Kind() == reflect.Pointer, dates: true}
		memo[original] = plan
		return plan, nil
	}
	if ownsJSONRepresentation(original) {
		return nil, nil
	}
	plan := &internalPlan{kind: target.Kind(), pointer: original.Kind() == reflect.Pointer}
	if target.Kind() == reflect.Array {
		plan.arrayLen = target.Len()
	}
	memo[original] = plan
	switch target.Kind() {
	case reflect.Slice, reflect.Array, reflect.Map:
		child, err := buildInternalPlan(target.Elem(), memo)
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
				item.child, err = buildInternalPlan(field.Field.ReflectedType, memo)
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
	return p != nil && (p.internal || normalizeDates && p.dates)
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
func filterPublic(raw []byte, target reflect.Type, normalizeDates bool) ([]byte, error) {
	plan, err := compileInternalPlan(target, map[reflect.Type]*internalPlan{})
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
