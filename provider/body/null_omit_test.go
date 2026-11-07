package body

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

type nullOmitRow struct {
	Value   float64                                       `json:"value" jsonx:"null=omit"`
	Pointer *float64                                      `json:"pointer" jsonx:"null=omit"`
	Plain   *float64                                      `json:"plain"`
	Secret  float64                                       `json:"secret" internal:"true" jsonx:"null=omit"`
	Has     *struct{ Value, Pointer, Plain, Secret bool } `json:"-" setMarker:"true"`
}

func TestNullOmitFloatNativeBody(t *testing.T) {
	for _, tc := range []struct {
		raw              string
		value            float64
		present, failure bool
	}{
		{`{}`, 0, false, false}, {`{"value":null}`, 0, false, false}, {`{"value":0}`, 0, true, false},
		{`{"value":1.5}`, 1.5, true, false}, {`{"value":1.5,"value":null}`, 1.5, true, false},
		{`{"value":null,"value":2.5}`, 2.5, true, false}, {`{"value":1,"value":2}`, 2, true, false},
		{`{"value":true}`, 0, false, true}, {`{"value":"1"}`, 0, false, true}, {`{"value":{},"value":2}`, 0, false, true},
	} {
		s, _ := New([]byte(tc.raw), "application/json", nil)
		v, _, err := s.Value(t.Context(), reflect.TypeFor[nullOmitRow](), "")
		if tc.failure {
			if err == nil {
				t.Errorf("accepted %s", tc.raw)
			}
			continue
		}
		if err != nil {
			t.Fatalf("%s: %v", tc.raw, err)
		}
		row := v.(nullOmitRow)
		if row.Value != tc.value || row.Has == nil || row.Has.Value != tc.present {
			t.Fatalf("%s => %+v", tc.raw, row)
		}
	}
}

func TestNullOmitNestedPointersAndCapture(t *testing.T) {
	type envelope struct {
		Rows  []*nullOmitRow         `json:"rows"`
		Named map[string]nullOmitRow `json:"named"`
	}
	raw := []byte(`{"data":{"rows":[{"pointer":null,"plain":null,"secret":4,"Has":{"Secret":true}},null,{"pointer":0}],"named":{"a":{"value":null}}}}`)
	s, _ := New(raw, "application/json", nil)
	v, _, err := s.Value(t.Context(), reflect.TypeFor[envelope](), "data")
	if err != nil {
		t.Fatal(err)
	}
	got := v.(envelope)
	first := got.Rows[0]
	if first.Has == nil || got.Rows[2].Has == nil {
		t.Fatalf("missing rows presence: first=%+v last=%+v", first, got.Rows[2])
	}
	if first.Pointer != nil || first.Plain != nil || first.Has.Pointer || !first.Has.Plain || first.Has.Secret || first.Secret != 0 || got.Rows[1] != nil || got.Rows[2].Pointer == nil || *got.Rows[2].Pointer != 0 || !got.Rows[2].Has.Pointer || (got.Named["a"].Has != nil && got.Named["a"].Has.Value) {
		t.Fatalf("unexpected result %+v", got)
	}
	captured, _, err := s.CaptureSource(t.Context(), reflect.TypeFor[envelope](), "data")
	if err != nil || !strings.Contains(string(captured.(json.RawMessage)), `"pointer":null`) {
		t.Fatalf("capture %s %v", captured, err)
	}
}
