package body_test

import (
	"context"
	"reflect"
	"testing"

	bodyprovider "github.com/viant/bindly/provider/body"
)

func TestSourceDecodesTypedJSONAndPresence(t *testing.T) {
	type has struct{ Active bool }
	type payload struct {
		Active bool `json:"active"`
		Has    *has `setMarker:"true"`
	}
	source, err := bodyprovider.New([]byte(`{"active":true}`), "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	value, found, err := source.Value(context.Background(), reflect.TypeOf(payload{}), "")
	if err != nil || !found {
		t.Fatalf("Value() = (%#v, %v, %v)", value, found, err)
	}
	actual := value.(payload)
	if !actual.Active || actual.Has == nil || !actual.Has.Active {
		t.Fatalf("payload = %+v", actual)
	}
}

func TestSourceSupportsExactNamedJSONLookup(t *testing.T) {
	raw := []byte(`{"Name":"Ada"}`)
	folded, err := bodyprovider.New(raw, "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	value, found, err := folded.Value(context.Background(), reflect.TypeOf(""), "name")
	if err != nil || !found || value != "Ada" {
		t.Fatalf("folded Value() = (%#v, %v, %v)", value, found, err)
	}
	exact, err := bodyprovider.New(raw, "application/json", nil, bodyprovider.WithExactFieldNames())
	if err != nil {
		t.Fatal(err)
	}
	if value, found, err = exact.Value(context.Background(), reflect.TypeOf(""), "name"); err != nil || found || value != nil {
		t.Fatalf("exact miss Value() = (%#v, %v, %v)", value, found, err)
	}
	value, found, err = exact.Value(context.Background(), reflect.TypeOf(""), "Name")
	if err != nil || !found || value != "Ada" {
		t.Fatalf("exact Value() = (%#v, %v, %v)", value, found, err)
	}
}
