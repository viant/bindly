package body

import (
	"context"
	"reflect"
	"testing"
)

func TestBodySource(t *testing.T) {
	for _, tt := range []struct {
		name, field  string
		exact, found bool
		want         any
	}{
		{"named", "Name", false, true, "Alice"},
		{"folded", "name", false, true, "Alice"},
		{"exact missing", "name", true, false, nil},
		{"exact named", "Name", true, true, "Alice"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var opts []Option
			if tt.exact {
				opts = append(opts, WithExactFieldNames())
			}
			source, err := New([]byte(`{"Name":"Alice"}`), "application/json", nil, opts...)
			if err != nil {
				t.Fatal(err)
			}
			value, found, err := source.Value(context.Background(), reflect.TypeOf(""), tt.field)
			if err != nil || found != tt.found || value != tt.want {
				t.Fatalf("value %v found %v err %v", value, found, err)
			}
		})
	}
}

func TestBodyShapes(t *testing.T) {
	type nested struct {
		Name string `json:"name"`
	}
	for _, tt := range []struct {
		name, raw, field string
		target           reflect.Type
		want             any
		reject           bool
	}{
		{"null", "null", "", reflect.TypeOf(0), nil, false},
		{"array", "[1,2]", "", reflect.TypeOf([]int{}), []int{1, 2}, false},
		{"named array", `{"items":[1,2]}`, "items", reflect.TypeOf([]int{}), []int{1, 2}, false},
		{"nested", `{"item":{"name":"Alice"}}`, "item", reflect.TypeOf(nested{}), nested{Name: "Alice"}, false},
		{"numeric string strict", `{"id":"1"}`, "id", reflect.TypeOf(0), nil, true},
		{"named null", `{"id":null}`, "id", reflect.TypeOf(0), nil, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			source, err := New([]byte(tt.raw), "application/json", nil)
			if err != nil {
				t.Fatal(err)
			}
			actual, found, err := source.Value(context.Background(), tt.target, tt.field)
			if tt.reject {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil || !found || !reflect.DeepEqual(actual, tt.want) {
				t.Fatalf("value %#v found %v error %v", actual, found, err)
			}
		})
	}
}

func TestBodyPresenceMarkers(t *testing.T) {
	type marker struct{ Name, Notes, Children bool }
	type childMarker struct{ Name bool }
	type child struct {
		Name string       `json:"name"`
		Has  *childMarker `setMarker:"true" json:"-"`
	}
	type record struct {
		Name     string  `json:"name"`
		Notes    *string `json:"notes"`
		Children []child `json:"children"`
		Has      *marker `setMarker:"true" json:"-"`
	}
	for _, tt := range []struct {
		name, raw       string
		notes, children bool
	}{
		{"omitted", `{"name":""}`, false, false},
		{"null", `{"name":"","notes":null}`, true, false},
		{"nested", `{"name":"","children":[{"name":"child"}]}`, false, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			source, err := New([]byte(tt.raw), "application/json", nil)
			if err != nil {
				t.Fatal(err)
			}
			value, _, err := source.Value(context.Background(), reflect.TypeOf(record{}), "")
			if err != nil {
				t.Fatal(err)
			}
			actual := value.(record)
			if actual.Has == nil || !actual.Has.Name || actual.Has.Notes != tt.notes || actual.Has.Children != tt.children {
				t.Fatalf("presence %+v", actual.Has)
			}
			if tt.children && (actual.Children[0].Has == nil || !actual.Children[0].Has.Name) {
				t.Fatal("nested presence missing")
			}
		})
	}
}
