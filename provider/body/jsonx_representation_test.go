package body

import (
	"github.com/viant/x/shape"
	"reflect"
	"testing"
)

// The same linked type can carry distinct effective tags on different routes.
func TestJSONXEffectiveRepresentationIsolation(t *testing.T) {
	type record struct {
		Flag *int                 `json:"flag"`
		Has  *struct{ Flag bool } `json:"-" setMarker:"true"`
	}
	typ := reflect.TypeFor[record]()
	plan, err := shape.CompileRewrite(typ)
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := plan.RewriteRepresentation(func(_ string, parent reflect.Type, field *shape.FieldMetadata) (*shape.FieldMetadata, error) {
		if parent == typ && field.Name == "Flag" {
			return &shape.FieldMetadata{Name: field.Name, Tag: `json:"enabled" jsonx:"boolToInt,null=omit"`}, nil
		}
		return nil, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for iteration := 0; iteration < 2; iteration++ {
		for _, tc := range []struct {
			raw     string
			present bool
			want    int
		}{
			{`{"enabled":true}`, true, 1},
			{`{"enabled":false}`, true, 0},
			{`{"enabled":null}`, false, 0},
		} {
			source, err := New([]byte(tc.raw), "application/json", nil, WithRepresentation(metadata))
			if err != nil {
				t.Fatal(err)
			}
			value, found, err := source.Value(t.Context(), typ, "")
			if err != nil || !found {
				t.Fatalf("%s: found=%v err=%v", tc.raw, found, err)
			}
			got := value.(record)
			if (got.Flag != nil) != tc.present || got.Flag != nil && *got.Flag != tc.want || got.Has == nil || got.Has.Flag != tc.present {
				t.Fatalf("%s: flag=%v presence=%+v", tc.raw, got.Flag, got.Has)
			}
		}
		plain, err := New([]byte(`{"flag":true}`), "application/json", nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err = plain.Value(t.Context(), typ, ""); err == nil {
			t.Fatal("route-specific boolean conversion leaked into untagged type")
		}
	}
}
