package bodyjson

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

type diagnosticValue struct {
	Name string `json:"name"`
}
type diagnosticRow struct {
	Value  *diagnosticValue `json:"value"`
	Nested struct {
		Value *diagnosticValue `json:"value"`
	} `json:"nested"`
}

func TestDefaultArrayTypeErrorIndex(t *testing.T) {
	type useCase struct{ description, input, expected string }
	for _, c := range []useCase{
		{"first record whitespace boundaries", ` [ {"value":"bad"} ] `, "0.value"},
		{"second record after null", "[null,\n {\"value\":\"bad\"}]", "1.value"},
		{"nested field suffix", `[{}, {"nested":{"value":"bad"}}]`, "1.nested.value"},
	} {
		t.Run(c.description, func(t *testing.T) {
			typ := reflect.TypeFor[[]*diagnosticRow]()
			var original []*diagnosticRow
			err := json.Unmarshal([]byte(c.input), &original)
			var before *json.UnmarshalTypeError
			if !errors.As(err, &before) {
				t.Fatal(err)
			}
			saved := *before
			_, err = DecodePublic([]byte(c.input), typ)
			var got *json.UnmarshalTypeError
			if !errors.As(err, &got) {
				t.Fatal(err)
			}
			if got.Field != c.expected || got.Value != saved.Value || got.Type != saved.Type || got.Offset != saved.Offset || got.Struct != saved.Struct {
				t.Fatalf("changed diagnostic: %#v before %#v", got, saved)
			}
			// Enrichment is idempotent and retains the same native error object.
			enrichArrayTypeError([]byte(c.input), typ, got)
			if got.Field != c.expected {
				t.Fatal(got.Field)
			}
		})
	}
}
func TestArrayTypeErrorNegative(t *testing.T) {
	type useCase struct {
		description, input string
		target             reflect.Type
	}
	for _, c := range []useCase{
		{"nonarray target", `{"value":"bad"}`, reflect.TypeFor[diagnosticRow]()},
		{"syntax failure", `[{"value":`, reflect.TypeFor[[]diagnosticRow]()},
		{"scalar element without field", `["bad"]`, reflect.TypeFor[[]diagnosticRow]()},
	} {
		t.Run(c.description, func(t *testing.T) {
			_, err := DecodePublic([]byte(c.input), c.target)
			if err == nil {
				t.Fatal("accepted invalid JSON")
			}
			var typed, original *json.UnmarshalTypeError
			standard := json.Unmarshal([]byte(c.input), reflect.New(c.target).Interface())
			if errors.As(err, &typed) && (!errors.As(standard, &original) || typed.Field != original.Field) {
				t.Fatalf("unrelated error enriched: %#v", typed)
			}
		})
	}
	custom := &json.UnmarshalTypeError{Field: "value", Type: reflect.TypeFor[diagnosticValue](), Offset: 15, Value: "string"}
	saved := *custom
	enrichArrayTypeError([]byte(`[{"value":"bad"}]`), reflect.TypeFor[opaqueDiagnostic](), custom)
	if *custom != saved {
		t.Fatal("custom owner changed")
	}
}

type opaqueDiagnostic []diagnosticRow

func (*opaqueDiagnostic) UnmarshalJSON([]byte) error { return errors.New("custom") }

func TestArrayTypeErrorCaseFoldedField(t *testing.T) {
	type row struct{ Value *diagnosticValue }
	type useCase struct{ description, input, expected string }
	for _, tc := range []useCase{
		{"lowercase first", `[{"value":"bad"}]`, "0.Value"},
		{"mixed case second", `[{}, {"vAlUe":"bad"}]`, "1.Value"},
		{"duplicate aliases remain ambiguous", `[{"value":"bad","Value":"also bad"}]`, "Value"},
	} {
		t.Run(tc.description, func(t *testing.T) {
			var rows []*row
			err := json.Unmarshal([]byte(tc.input), &rows)
			var failure *json.UnmarshalTypeError
			if !errors.As(err, &failure) {
				t.Fatal(err)
			}
			// Exercise the Go 1.26 field spelling even on newer toolchains.
			failure.Field = "Value"
			before := *failure
			enrichArrayTypeError([]byte(tc.input), reflect.TypeFor[[]*row](), failure)
			if failure.Field != tc.expected || failure.Offset != before.Offset || failure.Type != before.Type || failure.Value != before.Value {
				t.Fatalf("diagnostic=%#v want field %s", failure, tc.expected)
			}
		})
	}
}
