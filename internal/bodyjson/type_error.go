package bodyjson

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
)

// enrichArrayTypeError supplies an omitted outer index only when the exact
// standard-decoder bytes and native field ownership establish its provenance.
func enrichArrayTypeError(raw []byte, target reflect.Type, err error) {
	failure, ok := err.(*json.UnmarshalTypeError)
	if !ok || failure.Field == "" || failure.Offset <= 0 || ownsJSONRepresentation(target) {
		return
	}
	for target.Kind() == reflect.Pointer {
		target = target.Elem()
	}
	if target.Kind() != reflect.Slice && target.Kind() != reflect.Array {
		return
	}
	plan, e := compileInternalPlan(target.Elem(), nil)
	if e != nil || plan == nil {
		return
	}
	// Numeric prefixes are already supplied by newer standard decoders. A
	// literal dotted JSON member is ambiguous and is intentionally left alone.
	parts := strings.Split(failure.Field, ".")
	if _, e := strconv.Atoi(parts[0]); e == nil {
		return
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	token, e := d.Token()
	if e != nil || token != json.Delim('[') {
		return
	}
	for index := 0; d.More(); index++ {
		var item json.RawMessage
		if d.Decode(&item) != nil {
			return
		}
		end := d.InputOffset()
		start := end - int64(len(item))
		if failure.Offset <= start || failure.Offset > end {
			continue
		}
		if target.Kind() == reflect.Array && index >= target.Len() {
			return
		}
		if verifiedTypeErrorField(item, start, plan, parts, failure) {
			failure.Field = strconv.Itoa(index) + "." + failure.Field
		}
		return
	}
}

func verifiedTypeErrorField(raw []byte, base int64, plan *internalPlan, parts []string, failure *json.UnmarshalTypeError) bool {
	if plan == nil || plan.kind != reflect.Struct || len(parts) == 0 {
		return false
	}
	field := plan.field(parts[0])
	if field == nil {
		return false
	}
	// A custom representation owner must retain its own diagnostic semantics.
	typ := field.field.Field.ReflectedType
	if ownsJSONRepresentation(typ) {
		return false
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	token, e := d.Token()
	if e != nil || token != json.Delim('{') {
		return false
	}
	matches := 0
	verified := false
	for d.More() {
		key, e := d.Token()
		if e != nil {
			return false
		}
		var value json.RawMessage
		if d.Decode(&value) != nil {
			return false
		}
		end := base + d.InputOffset()
		start := end - int64(len(value))
		if key != field.field.Name {
			continue
		}
		matches++
		if failure.Offset <= start || failure.Offset > end {
			continue
		}
		if len(parts) == 1 {
			for typ.Kind() == reflect.Pointer {
				typ = typ.Elem()
			}
			verified = typ == failure.Type
		} else {
			verified = verifiedTypeErrorField(value, start, field.child, parts[1:], failure)
		}
	}
	return matches == 1 && verified
}
