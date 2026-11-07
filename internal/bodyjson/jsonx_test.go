package bodyjson

import (
	"reflect"
	"testing"
)

func TestInputRuleScope(t *testing.T) {
	for _, typ := range []reflect.Type{reflect.TypeFor[int](), reflect.TypeFor[*int]()} {
		if _, err := compileInputRule(`jsonx:"boolToInt,null=omit"`, typ); err != nil {
			t.Fatalf("supported %v: %v", typ, err)
		}
	}
	for _, typ := range []reflect.Type{reflect.TypeFor[int64](), reflect.TypeFor[uint](), reflect.TypeFor[float64](), reflect.TypeFor[string]()} {
		if _, err := compileInputRule(`jsonx:"boolToInt,null=omit"`, typ); err == nil {
			t.Errorf("unexpected policy support for %v", typ)
		}
		if _, err := compileInputRule(`json:"value"`, typ); err != nil {
			t.Errorf("untagged %v changed: %v", typ, err)
		}
	}
}

func TestNullOmitTypeScope(t *testing.T) {
	for _, typ := range []reflect.Type{reflect.TypeFor[float64](), reflect.TypeFor[*float64](), reflect.TypeFor[string](), reflect.TypeFor[[]string](), reflect.TypeFor[map[string]int](), reflect.TypeFor[struct{ Value int }]()} {
		if _, err := compileInputRule(`jsonx:"null=omit"`, typ); err != nil {
			t.Errorf("%v: %v", typ, err)
		}
	}
	if _, err := compileInputRule(`jsonx:"null=omit"`, reflect.TypeFor[opaqueDiagnostic]()); err == nil {
		t.Error("custom JSON owner overridden")
	}
}
