package bodyjson

import (
	"bytes"
	"fmt"
	"reflect"
	"strings"
)

// inputRule augments native decoding only on explicitly authored integer fields.
type inputRule struct{ boolToInt, omitNull bool }

func compileInputRule(tag reflect.StructTag, typ reflect.Type) (inputRule, error) {
	var rule inputRule
	for _, item := range strings.Split(tag.Get("jsonx"), ",") {
		switch strings.TrimSpace(item) {
		case "boolToInt":
			rule.boolToInt = true
		case "null=omit":
			rule.omitNull = true
		}
	}
	if !rule.boolToInt && !rule.omitNull {
		return rule, nil
	}
	if ownsJSONRepresentation(typ) {
		return rule, fmt.Errorf("jsonx input rules cannot override custom JSON ownership on %v", typ)
	}
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	switch typ.Kind() {
	case reflect.Int:
		return rule, nil
	}
	return rule, fmt.Errorf("jsonx boolToInt/null=omit requires an int field, got %v", typ)
}
func (r inputRule) active() bool { return r.boolToInt || r.omitNull }
func (r inputRule) apply(raw []byte) ([]byte, bool) {
	token := bytes.TrimSpace(raw)
	if r.omitNull && bytes.Equal(token, []byte("null")) {
		return nil, true
	}
	if r.boolToInt {
		if bytes.Equal(token, []byte("true")) {
			return []byte("1"), false
		}
		if bytes.Equal(token, []byte("false")) {
			return []byte("0"), false
		}
	}
	return raw, false
}
