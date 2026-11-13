package body

import (
	"context"
	"github.com/viant/tagly/choice"
	"reflect"
	"testing"
)

func TestRequiredPresenceIsExplicitAndNotTruthiness(t *testing.T) {
	type item struct {
		Value    int    `json:"value" required:"true"`
		Flag     bool   `json:"flag" required:"true"`
		Hidden   string `json:"hidden" internal:"true" required:"true"`
		Optional string `json:"optional" required:"true" optional:"true"`
	}
	type root struct {
		Items  []*item `json:"items"`
		Detail *item   `json:"detail"`
	}
	for _, tc := range []struct {
		raw   string
		valid bool
	}{{`{}`, true}, {`{"items":[{"value":0,"flag":false}]}`, true}, {`{"detail":null}`, true}, {`{"detail":{"value":null,"flag":false}}`, true}, {`{"items":[{"value":1}]}`, false}, {`{"detail":{}}`, false}, {`{"items":[null,{"VALUE":0,"FLAG":false}]}`, true}} {
		err := ValidateRequired([]byte(tc.raw), reflect.TypeOf(root{}))
		if (err == nil) != tc.valid {
			t.Fatalf("%s: %v", tc.raw, err)
		}
	}
	// Existing native decode remains unchanged without the opt-in check.
	source, _ := New([]byte(`{}`), "application/json", nil)
	if _, _, err := source.Value(context.Background(), reflect.TypeOf(item{}), ""); err != nil {
		t.Fatal(err)
	}
}

func TestRequiredPresenceUsesScalarChoiceWireShape(t *testing.T) {
	type fee struct {
		Type *choice.ScalarValue `json:"feeTypeId" required:"true" choice:"4237:AD_SERVING" enumAccept:"scalar" enumStyle:"scalar" enumInteger:"passthrough"`
	}
	target := reflect.TypeFor[[]fee]()
	for _, tc := range []struct {
		raw   string
		valid bool
	}{
		{`[{"feeTypeId":"AD_SERVING"}]`, true},
		{`[{"feeTypeId":4237}]`, true},
		{`[{"feeTypeId":null}]`, true},
		{`[{}]`, false},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			if err := ValidateRequired([]byte(tc.raw), target); (err == nil) != tc.valid {
				t.Fatalf("required presence: %v", err)
			}
		})
	}
}
