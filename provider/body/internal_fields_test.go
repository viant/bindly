package body

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

type publicInternalHas struct{ Name, Secret, Off, Zero, Aux, Child, Items, Map, Raw, Binary, Big, Custom bool }
type publicInternalRecord struct {
	Name   string                           `json:"name"`
	Secret string                           `internal:"true"`
	Off    bool                             `internal:"true"`
	Zero   int                              `internal:"true"`
	Aux    *internalDecoderSpy              `internal:"true"`
	Child  *publicInternalRecord            `json:"child,omitempty"`
	Items  []*publicInternalRecord          `json:"items,omitempty"`
	Map    map[string]*publicInternalRecord `json:"map,omitempty"`
	Raw    json.RawMessage                  `json:"raw,omitempty"`
	Binary []byte                           `json:"binary,omitempty"`
	Big    int64                            `json:"big,omitempty"`
	Custom *publicDecoderSpy                `json:"custom,omitempty"`
	Has    *publicInternalHas               `json:"-" setMarker:"true"`
}

var internalDecoderCalls int

type internalDecoderSpy struct{ Value string }

func (s *internalDecoderSpy) UnmarshalJSON([]byte) error {
	internalDecoderCalls++
	s.Value = "decoded"
	return nil
}

type publicDecoderSpy struct{ Raw string }

func (s *publicDecoderSpy) UnmarshalJSON(raw []byte) error { s.Raw = string(raw); return nil }

func TestPublicInternalFieldsExcludedBeforeDecoding(t *testing.T) {
	for _, raw := range []string{
		`{"name":"ok","Secret":"forged","Off":true,"Zero":7,"Aux":{},"Has":{"Secret":true,"Name":true}}`,
		`{"name":"ok","secret":null,"OFF":false,"zero":0,"aux":{},"Secret":"again"}`,
		`{"name":"ok","Secret":7,"Aux":[1,2]}`,
	} {
		t.Run(raw, func(t *testing.T) {
			internalDecoderCalls = 0
			s, _ := New([]byte(raw), "application/json", nil)
			value, found, err := s.Value(context.Background(), reflect.TypeFor[publicInternalRecord](), "")
			if err != nil || !found {
				t.Fatalf("decode %v", err)
			}
			row := value.(publicInternalRecord)
			if row.Name != "ok" || row.Secret != "" || row.Off || row.Zero != 0 || row.Aux != nil || internalDecoderCalls != 0 {
				t.Fatalf("internal input decoded: %+v spy=%d", row, internalDecoderCalls)
			}
			if row.Has == nil || !row.Has.Name || row.Has.Secret || row.Has.Off || row.Has.Zero || row.Has.Aux {
				t.Fatalf("client presence %+v", row.Has)
			}
		})
	}
}

func TestPublicInternalNestedAndLexicalJSON(t *testing.T) {
	raw := `{"name":"first","Secret":"drop","Name":"last","big":9007199254740993,"raw":{"n":9007199254740993,"n":1e200},"binary":"AAH/","custom":{"n":9007199254740993,"n":2},"child":{"Secret":"drop","name":"child"},"items":[{"name":"item","Secret":"drop"},null],"map":{"one":{"name":"mapped","Secret":"drop","Has":{"Name":true,"Secret":true}}},"unknown":{"Secret":"data"}}`
	source, _ := New([]byte(raw), "application/problem+json", nil)
	value, _, err := source.Value(context.Background(), reflect.TypeFor[*publicInternalRecord](), "")
	if err != nil {
		t.Fatal(err)
	}
	row := value.(*publicInternalRecord)
	if row.Name != "last" || row.Big != 9007199254740993 || string(row.Raw) != `{"n":9007199254740993,"n":1e200}` || !reflect.DeepEqual(row.Binary, []byte{0, 1, 255}) || row.Custom.Raw != `{"n":9007199254740993,"n":2}` {
		t.Fatalf("public lexical values changed %+v", row)
	}
	if row.Child.Secret != "" || row.Items[0].Secret != "" || row.Items[1] != nil || row.Map["one"].Secret != "" {
		t.Fatal("nested hidden values survived")
	}
	// Native presence does not traverse maps. Generated json:"-" markers
	// still reject forged holders, and this change does not add map presence.
	if row.Map["one"].Has != nil {
		t.Fatal("forged map child marker decoded")
	}
	if row.Child.Has == nil || !row.Child.Has.Name || row.Child.Has.Secret || row.Items[0].Has == nil || row.Items[0].Has.Secret {
		t.Fatal("nested markers lost/forged")
	}
	captured, _, err := source.CaptureSource(context.Background(), reflect.TypeFor[*publicInternalRecord](), "")
	if err != nil || string(captured.(json.RawMessage)) != raw {
		t.Fatal("raw capture changed", err)
	}
}

type internalDominanceBase struct{ Name string }
type internalDominanceWinner struct {
	internalDominanceBase
	Name string               `internal:"true"`
	Has  *struct{ Name bool } `setMarker:"true" json:"-"`
}
type internalPromotedOwner struct {
	internalDominanceBase `internal:"true"`
	Public                string
	Has                   *struct{ Name, Public bool } `setMarker:"true" json:"-"`
}
type internalFoldedRecord struct {
	Public string                         `json:"NAME"`
	Secret string                         `json:"Name" internal:"true"`
	Has    *struct{ Public, Secret bool } `setMarker:"true" json:"-"`
}

func TestPublicInternalCanonicalDominanceAndOwners(t *testing.T) {
	s, _ := New([]byte(`{"Name":"forged","Public":"ok"}`), "application/json", nil)
	value, _, err := s.Value(context.Background(), reflect.TypeFor[internalDominanceWinner](), "")
	if err != nil {
		t.Fatal(err)
	}
	winner := value.(internalDominanceWinner)
	if winner.Name != "" || winner.internalDominanceBase.Name != "" || winner.Has.Name {
		t.Fatalf("shadowed public loser exposed %+v", winner)
	}
	value, _, err = s.Value(context.Background(), reflect.TypeFor[internalPromotedOwner](), "")
	if err != nil {
		t.Fatal(err)
	}
	owner := value.(internalPromotedOwner)
	if owner.Name != "" || owner.Public != "ok" || owner.Has.Name || !owner.Has.Public {
		t.Fatalf("internal embedding exposed %+v", owner)
	}
	for _, raw := range []string{`{"NAME":"public","Name":"hidden"}`, `{"name":"public","Name":"hidden"}`, `{"Name":"hidden","name":"public"}`} {
		s, _ = New([]byte(raw), "application/json", nil)
		value, _, err = s.Value(context.Background(), reflect.TypeFor[internalFoldedRecord](), "")
		if err != nil {
			t.Fatal(err)
		}
		record := value.(internalFoldedRecord)
		if record.Public != "public" || record.Secret != "" || !record.Has.Public || record.Has.Secret {
			t.Fatalf("canonical aliases %+v for %s", record, raw)
		}
	}
}

func TestPublicInternalUnknownJSONErrorsNullAndDeferred(t *testing.T) {
	for _, raw := range []string{`{"Secret":{"bad":},"name":"ok"}`, `{"Secret":1} garbage`, `{"Secret":1} {}`} {
		s, _ := New([]byte(raw), "application/json", nil)
		if _, _, err := s.Value(context.Background(), reflect.TypeFor[publicInternalRecord](), ""); err == nil {
			t.Fatalf("malformed hidden input accepted %s", raw)
		}
	}
	for _, raw := range []string{`{"Secret":1}`, `{"Secret":1,"name":"ok"}`, `{"name":"ok","Secret":1}`, `{"Secret":1,"name":"ok","Off":true}`} {
		s, _ := New([]byte(raw), "application/json", nil)
		if _, _, err := s.Value(context.Background(), reflect.TypeFor[publicInternalRecord](), ""); err != nil {
			t.Fatalf("removed member delimiters: %s %v", raw, err)
		}
	}
	loads := 0
	s := NewDeferred(func(context.Context) (*Source, error) {
		loads++
		return New([]byte(`{"data":{"name":"ok","Secret":"forged"}}`), "application/json", map[string]string{"row": "data"})
	})
	for i := 0; i < 2; i++ {
		value, found, err := s.Value(context.Background(), reflect.TypeFor[*publicInternalRecord](), "row")
		if err != nil || !found || value.(*publicInternalRecord).Secret != "" {
			t.Fatal("deferred/alias filtering", err)
		}
	}
	if loads != 1 {
		t.Fatal("deferred loaded repeatedly")
	}
	s, _ = New([]byte(`null`), "application/json", nil)
	value, found, err := s.ValueWithBodyNullPolicy(context.Background(), reflect.TypeFor[*publicInternalRecord](), "", "empty-record")
	if err != nil || !found || value.(*publicInternalRecord).Has == nil || value.(*publicInternalRecord).Has.Secret {
		t.Fatal("empty record policy", err)
	}
	value, found, err = s.Value(context.Background(), reflect.TypeFor[*publicInternalRecord](), "")
	if err != nil || !found || value != nil {
		t.Fatal("ordinary null changed", err)
	}
	raw := ` {"Secret":123} `
	s, _ = New([]byte(raw), "application/json", nil)
	value, _, err = s.Value(context.Background(), reflect.TypeFor[json.RawMessage](), "")
	if err != nil || string(value.(json.RawMessage)) != raw {
		t.Fatal("raw value changed", err)
	}
}

type opaqueBodyWithInternal struct {
	Secret string `internal:"true"`
}

func (o *opaqueBodyWithInternal) UnmarshalJSON(raw []byte) error { o.Secret = string(raw); return nil }
func TestPublicOpaqueJSONAuthorityUnchanged(t *testing.T) {
	s, _ := New([]byte(`{"Secret":"custom authority"}`), "application/json", nil)
	value, _, err := s.Value(context.Background(), reflect.TypeFor[*opaqueBodyWithInternal](), "")
	if err != nil || !strings.Contains(value.(*opaqueBodyWithInternal).Secret, "custom authority") {
		t.Fatal("opaque representation was structurally changed", err)
	}
}

type publicTextJSON string

func (s *publicTextJSON) UnmarshalText(raw []byte) error {
	*s = publicTextJSON("text:" + string(raw))
	return nil
}
func TestPublicInternalNativeScalarContracts(t *testing.T) {
	type row struct {
		Secret string                                                           `internal:"true"`
		Public string                                                           `internal:"false"`
		Other  string                                                           `internal:"yes"`
		Count  int64                                                            `json:"count,string"`
		Date   time.Time                                                        `json:"date"`
		Text   publicTextJSON                                                   `json:"text"`
		Hidden string                                                           `json:"-"`
		Has    *struct{ Secret, Public, Other, Count, Date, Text, Hidden bool } `json:"-" setMarker:"true"`
	}
	source, _ := New([]byte(`{"Secret":[],"Public":"visible","Other":"also visible","count":"9007199254740993","date":"2026-10-05T00:00:00Z","text":"native","Hidden":"forged"}`), "application/json", nil)
	value, _, err := source.Value(context.Background(), reflect.TypeFor[row](), "")
	if err != nil {
		t.Fatal(err)
	}
	actual := value.(row)
	if actual.Secret != "" || actual.Public != "visible" || actual.Other != "also visible" || actual.Count != 9007199254740993 || !actual.Date.Equal(time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)) || actual.Text != "text:native" || actual.Hidden != "" || actual.Has.Secret || actual.Has.Hidden || !actual.Has.Public || !actual.Has.Other || !actual.Has.Count || !actual.Has.Date || !actual.Has.Text {
		t.Fatalf("native public scalar semantics changed: %+v Has=%+v", actual, actual.Has)
	}
	bad, _ := New([]byte(`{"Secret":1,"count":"invalid"}`), "application/json", nil)
	if _, _, err = bad.Value(context.Background(), reflect.TypeFor[row](), ""); err == nil {
		t.Fatal("public scalar type error suppressed")
	}
}

type internalPresenceAliasChild struct {
	A, B int
	Has  *struct{ A, B bool } `json:"-" setMarker:"true"`
}
type internalPresenceAliasControl struct {
	Child  *internalPresenceAliasChild   `json:"child"`
	Items  []*internalPresenceAliasChild `json:"items"`
	Alias  *internalPresenceAliasChild   `json:"wireName"`
	Secret string
	Has    *struct{ Child, Items, Alias, Secret bool } `json:"-" setMarker:"true"`
}
type internalPresenceAliasFiltered struct {
	Child  *internalPresenceAliasChild                 `json:"child"`
	Items  []*internalPresenceAliasChild               `json:"items"`
	Alias  *internalPresenceAliasChild                 `json:"wireName"`
	Secret string                                      `internal:"true"`
	Has    *struct{ Child, Items, Alias, Secret bool } `json:"-" setMarker:"true"`
}

func TestPublicInternalCanonicalPresenceMatchesNativeExactAliases(t *testing.T) {
	for _, raw := range []string{
		`{"child":{"A":1},"CHILD":{"B":2},"Secret":"drop"}`,
		`{"CHILD":{"B":2},"child":{"A":1},"Secret":"drop"}`,
		`{"items":[{"A":1}],"ITEMS":[{"B":2}],"Secret":"drop"}`,
		`{"ITEMS":[{"B":2}],"items":[{"A":1}],"Secret":"drop"}`,
		`{"wireName":{"A":1},"WIRENAME":{"B":2},"Secret":"drop"}`,
		`{"WIRENAME":{"B":2},"wireName":{"A":1},"Secret":"drop"}`,
		`{"child":{"A":1},"CHILD":null,"Secret":"drop"}`,
		`{"CHILD":{"B":2},"child":null,"Secret":"drop"}`,
		`{"child":null,"CHILD":{"B":2},"Secret":"drop"}`,
		`{"items":null,"ITEMS":[{"B":2}],"Secret":"drop"}`,
	} {
		t.Run(raw, func(t *testing.T) {
			source, _ := New([]byte(raw), "application/json", nil)
			controlValue, _, err := source.Value(context.Background(), reflect.TypeFor[internalPresenceAliasControl](), "")
			if err != nil {
				t.Fatal(err)
			}
			control := controlValue.(internalPresenceAliasControl)
			for iteration := 0; iteration < 1000; iteration++ {
				value, _, err := source.Value(context.Background(), reflect.TypeFor[internalPresenceAliasFiltered](), "")
				if err != nil {
					t.Fatal(err)
				}
				actual := value.(internalPresenceAliasFiltered)
				if !reflect.DeepEqual(actual.Child, control.Child) || !reflect.DeepEqual(actual.Items, control.Items) || !reflect.DeepEqual(actual.Alias, control.Alias) || actual.Has.Child != control.Has.Child || actual.Has.Items != control.Has.Items || actual.Has.Alias != control.Has.Alias || actual.Secret != "" || actual.Has.Secret {
					t.Fatalf("iteration %d canonical presence differs from unchanged native control: child=%+v controlChild=%+v items=%+v controlItems=%+v alias=%+v controlAlias=%+v Has=%+v controlHas=%+v", iteration, aliasPresenceSummary(actual.Child), aliasPresenceSummary(control.Child), aliasPresenceSummary(actual.Items...), aliasPresenceSummary(control.Items...), aliasPresenceSummary(actual.Alias), aliasPresenceSummary(control.Alias), actual.Has, control.Has)
				}
			}
		})
	}
}
func TestPublicInternalCanonicalPresenceFoldedWinnerOnly(t *testing.T) {
	type row struct {
		First  *internalPresenceAliasChild           `json:"NAME"`
		Second *internalPresenceAliasChild           `json:"Name"`
		Secret string                                `internal:"true"`
		Has    *struct{ First, Second, Secret bool } `json:"-" setMarker:"true"`
	}
	for _, raw := range []string{`{"name":{"A":1},"Secret":"drop"}`, `{"name":{"A":1},"Name":{"B":2},"Secret":"drop"}`, `{"Name":{"B":2},"name":{"A":1},"Secret":"drop"}`} {
		source, _ := New([]byte(raw), "application/json", nil)
		value, _, err := source.Value(context.Background(), reflect.TypeFor[row](), "")
		if err != nil {
			t.Fatal(err)
		}
		actual := value.(row)
		if actual.First == nil || actual.First.A != 1 || !actual.First.Has.A || actual.First.Has.B || !actual.Has.First || actual.Secret != "" || actual.Has.Secret {
			t.Fatalf("folded first winner wrong: %+v", actual)
		}
		hasSecond := strings.Contains(raw, `"Name"`)
		if actual.Has.Second != hasSecond || (!hasSecond && actual.Second != nil) || (hasSecond && (actual.Second.B != 2 || !actual.Second.Has.B || actual.Second.Has.A)) {
			t.Fatalf("another winner's folded key marked second field: %+v", actual)
		}
	}
}

func aliasPresenceSummary(rows ...*internalPresenceAliasChild) []struct {
	Nil             bool
	A, B            int
	Has, HasA, HasB bool
} {
	result := make([]struct {
		Nil             bool
		A, B            int
		Has, HasA, HasB bool
	}, len(rows))
	for i, row := range rows {
		if row == nil {
			result[i].Nil = true
			continue
		}
		result[i].A = row.A
		result[i].B = row.B
		if row.Has != nil {
			result[i].Has = true
			result[i].HasA = row.Has.A
			result[i].HasB = row.Has.B
		}
	}
	return result
}

func TestPublicInternalRegisteredJSONDecoderPreservesOwner(t *testing.T) {
	registry := NewDecoderRegistry()
	calls := 0
	registry.Register("application/json", DecoderFunc(func(raw []byte, target any) error {
		calls++
		var object map[string]json.RawMessage
		if err := json.Unmarshal(raw, &object); err != nil {
			return err
		}
		if _, found := object["Secret"]; found {
			t.Fatal("internal member reached registered JSON decoder")
		}
		return json.Unmarshal(raw, target)
	}))
	source, err := New([]byte(`{"name":"kept","Secret":"forged","Off":false,"Has":{"Secret":true}}`), "application/json", nil, WithDecoders(registry))
	if err != nil {
		t.Fatal(err)
	}
	value, found, err := source.Value(context.Background(), reflect.TypeFor[*publicInternalRecord](), "")
	if err != nil || !found || calls != 1 {
		t.Fatalf("registered JSON decoder: found=%v calls=%d err=%v", found, calls, err)
	}
	actual := value.(*publicInternalRecord)
	if actual.Name != "kept" || actual.Secret != "" || actual.Has == nil || !actual.Has.Name || actual.Has.Secret || actual.Has.Off {
		t.Fatalf("public presence lost: %+v", actual)
	}
}
