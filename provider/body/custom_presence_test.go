package body

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
)

type ownedPresenceFlags struct{ Value bool }
type ownedPresenceBody struct {
	Value *int                `json:"value" internal:"true"`
	Has   *ownedPresenceFlags `json:"-" setMarker:"true"`
}

// The authored decoder owns both its public alias and sparse marker; submitted
// Has/internal fields are deliberately absent from its wire contract.
func (b *ownedPresenceBody) UnmarshalJSON(raw []byte) error {
	var wire struct {
		Alias *int `json:"alias"`
	}
	if err := json.Unmarshal(raw, &wire); err != nil {
		return err
	}
	b.Value = wire.Alias
	b.Has = &ownedPresenceFlags{Value: wire.Alias != nil}
	return nil
}

func TestCustomJSONPreservesOwnedPresence(t *testing.T) {
	for _, target := range []reflect.Type{reflect.TypeFor[ownedPresenceBody](), reflect.TypeFor[*ownedPresenceBody]()} {
		t.Run(target.String(), func(t *testing.T) {
			s, err := New([]byte(`{"alias":0,"Has":{"Value":false}}`), "application/json", nil)
			if err != nil {
				t.Fatal(err)
			}
			got, found, err := s.Value(context.Background(), target, "")
			if err != nil || !found {
				t.Fatalf("decode: found=%v error=%v", found, err)
			}
			var row *ownedPresenceBody
			if v, ok := got.(*ownedPresenceBody); ok {
				row = v
			} else {
				v := got.(ownedPresenceBody)
				row = &v
			}
			if row.Value == nil || *row.Value != 0 || row.Has == nil || !row.Has.Value {
				t.Fatalf("authored decoder state overwritten: %+v flags=%+v", row, row.Has)
			}
		})
	}
}

func TestNestedCustomJSONPreservesOwnedPresence(t *testing.T) {
	type envelope struct {
		Items []*ownedPresenceBody  `json:"items"`
		Has   *struct{ Items bool } `json:"-" setMarker:"true"`
	}
	s, err := New([]byte(`{"items":[{"alias":0},null],"Has":{"Items":false}}`), "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	got, _, err := s.Value(context.Background(), reflect.TypeFor[envelope](), "")
	if err != nil {
		t.Fatal(err)
	}
	row := got.(envelope)
	if row.Has == nil || !row.Has.Items || len(row.Items) != 2 || row.Items[1] != nil {
		t.Fatalf("ordinary envelope presence changed: %+v", row)
	}
	if row.Items[0].Has == nil || !row.Items[0].Has.Value {
		t.Fatalf("nested authored marker overwritten: %+v", row.Items[0].Has)
	}
}

func TestRegisteredJSONPreservesOwnedPresence(t *testing.T) {
	registry := NewDecoderRegistry()
	calls := 0
	registry.Register("application/json", DecoderFunc(func(raw []byte, target any) error { calls++; return json.Unmarshal(raw, target) }))
	source, err := New([]byte(`{"alias":0,"Has":{"Value":false}}`), "application/json", nil, WithDecoders(registry))
	if err != nil {
		t.Fatal(err)
	}
	value, found, err := source.Value(context.Background(), reflect.TypeFor[*ownedPresenceBody](), "")
	if err != nil || !found {
		t.Fatalf("found=%v error=%v", found, err)
	}
	row := value.(*ownedPresenceBody)
	if calls != 1 || row.Value == nil || *row.Value != 0 || row.Has == nil || !row.Has.Value {
		t.Fatalf("registered decoder state overwritten: calls=%d row=%+v flags=%+v", calls, row, row.Has)
	}
}

type ownedPlainRow struct {
	Value *int                `json:"value" internal:"true"`
	Has   *ownedPresenceFlags `json:"-" setMarker:"true"`
}
type ownedEnvelope struct {
	Row *ownedPlainRow `json:"row"`
}

func (e *ownedEnvelope) UnmarshalJSON(raw []byte) error {
	var wire struct {
		Row struct {
			Alias *int `json:"alias"`
		} `json:"row"`
	}
	if err := json.Unmarshal(raw, &wire); err != nil {
		return err
	}
	e.Row = &ownedPlainRow{Value: wire.Row.Alias, Has: &ownedPresenceFlags{Value: wire.Row.Alias != nil}}
	return nil
}
func TestCustomJSONOwnsDescendantPresence(t *testing.T) {
	source, err := New([]byte(`{"row":{"alias":0,"Has":{"Value":false}}}`), "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	value, _, err := source.Value(context.Background(), reflect.TypeFor[*ownedEnvelope](), "")
	if err != nil {
		t.Fatal(err)
	}
	row := value.(*ownedEnvelope).Row
	if row == nil || row.Value == nil || *row.Value != 0 || row.Has == nil || !row.Has.Value {
		t.Fatalf("custom envelope descendant overwritten: %+v", row)
	}
}
