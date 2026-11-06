package body

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	structjson "github.com/viant/structology/encoding/json"
	"github.com/viant/x/shape"
)

type MetadataEmbedded struct {
	Public string `json:"public"`
}

func (v *MetadataEmbedded) Label() string { return v.Public }

type metadataLeaf struct {
	*MetadataEmbedded
	Value  int                                   `json:"value"`
	Secret string                                `json:"secret"`
	Has    *struct{ Public, Value, Secret bool } `setMarker:"true" json:"-"`
}
type metadataGroup struct {
	*MetadataEmbedded
	Leaves [][]*metadataLeaf                      `json:"leaves"`
	ByName map[string][]*metadataLeaf             `json:"byName"`
	Has    *struct{ Public, Leaves, ByName bool } `setMarker:"true" json:"-"`
}
type metadataRoot struct {
	Groups []*metadataGroup `json:"groups"`
	Direct *metadataLeaf    `json:"direct"`
}
type metadataNativeDecoder struct{}

func (metadataNativeDecoder) Decode(raw []byte, target any) error {
	return structjson.Unmarshal(raw, target)
}
func (metadataNativeDecoder) UnmarshalWithRepresentation(raw []byte, target any, metadata *shape.Representation) error {
	return structjson.Unmarshal(raw, target, structjson.WithRepresentation(metadata))
}
func leafMetadata(t *testing.T, typ reflect.Type) *shape.Representation {
	t.Helper()
	plan, err := shape.CompileRewrite(typ)
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := plan.RewriteRepresentation(func(_ string, parent reflect.Type, f *shape.FieldMetadata) (*shape.FieldMetadata, error) {
		if parent == reflect.TypeFor[metadataLeaf]() {
			switch f.Name {
			case "Value":
				return &shape.FieldMetadata{Name: "Renamed", Tag: `json:"amount" required:"true"`}, nil
			case "Secret":
				return &shape.FieldMetadata{Name: f.Name, Tag: `json:"-" internal:"true"`}, nil
			}
		}
		return nil, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return metadata
}
func TestRepresentationMultiLevelNestedAndEmbeddedTypes(t *testing.T) {
	metadata := leafMetadata(t, reflect.TypeFor[*metadataRoot]())
	raw := []byte(`{"groups":[{"public":"group","leaves":[[{"public":"leaf","amount":0,"secret":"blocked","Has":{"Secret":true}},null]],"byName":{"x":[{"amount":8,"secret":"blocked"}]}}],"direct":{"amount":3,"secret":"blocked"}}`)
	for _, mode := range []string{"default-standard", "registered-standard", "structology"} {
		t.Run(mode, func(t *testing.T) {
			options := []Option{WithRepresentation(metadata)}
			if mode != "default-standard" {
				decoders := NewDecoderRegistry()
				if mode == "structology" {
					decoders.Register("application/json", metadataNativeDecoder{})
				}
				options = append(options, WithDecoders(decoders))
			}
			if err := ValidateRequired(raw, metadata.Original(), metadata); err != nil {
				t.Fatal(err)
			}
			source, err := New(raw, "application/json", nil, options...)
			if err != nil {
				t.Fatal(err)
			}
			value, found, err := source.Value(t.Context(), metadata.Original(), "")
			if err != nil || !found {
				t.Fatalf("found=%v err=%v", found, err)
			}
			actual := value.(*metadataRoot)
			group := actual.Groups[0]
			leaf := group.Leaves[0][0]
			mapped := group.ByName["x"][0]
			if group.Label() != "group" || leaf.Label() != "leaf" {
				t.Fatal("linked embedded methods/storage lost")
			}
			if leaf.Value != 0 || mapped.Value != 8 || actual.Direct.Value != 3 || group.Leaves[0][1] != nil {
				t.Fatal("nested values/containers lost")
			}
			for _, item := range []*metadataLeaf{leaf, mapped, actual.Direct} {
				if item.Has == nil || !item.Has.Value || item.Has.Secret || item.Secret != "" {
					t.Fatalf("canonical presence or filtering lost: %+v", item)
				}
				item.Secret = "server-private"
			}
			if !leaf.Has.Public || group.Has == nil || !group.Has.Public || !group.Has.Leaves || !group.Has.ByName {
				t.Fatal("multi-level presence lost")
			}
			encoded, err := structjson.MarshalStandard(actual, structjson.WithRepresentation(metadata))
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(encoded), "secret") || strings.Contains(string(encoded), "server-private") {
				t.Fatal(string(encoded))
			}
			if leaf.Secret != "server-private" {
				t.Fatal("encoding mutated original value")
			}
			if err := ValidateRequired([]byte(`{"direct":{"value":1}}`), metadata.Original(), metadata); err == nil {
				t.Fatal("canonical alias bypassed effective requiredness")
			}
		})
	}
}
func TestRepresentationIntegerMapPresence(t *testing.T) {
	type root struct {
		ByID map[int][]*metadataLeaf `json:"byId"`
	}
	metadata := leafMetadata(t, reflect.TypeFor[*root]())
	source, err := New([]byte(`{"byId":{"02":[{"amount":0,"secret":"blocked"}],"2":[{"amount":9}]}}`), "application/json", nil, WithRepresentation(metadata))
	if err != nil {
		t.Fatal(err)
	}
	value, _, err := source.Value(t.Context(), metadata.Original(), "")
	if err != nil {
		t.Fatal(err)
	}
	leaf := value.(*root).ByID[2][0]
	if leaf.Value != 9 || leaf.Has == nil || !leaf.Has.Value || leaf.Has.Secret {
		t.Fatal("integer-map last-value presence lost")
	}
	encoded, err := json.Marshal(value)
	if err != nil || len(encoded) == 0 {
		t.Fatal(err)
	}
}

func TestLinkedMapWithoutMetadataRetainsNativeBehavior(t *testing.T) {
	type root struct {
		ByID map[int][]*metadataLeaf `json:"byId"`
	}
	source, err := New([]byte(`{"byId":{"5":[{"value":0,"secret":"allowed","Has":{"Public":true}}]}}`), "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	value, _, err := source.Value(t.Context(), reflect.TypeFor[*root](), "")
	if err != nil {
		t.Fatal(err)
	}
	leaf := value.(*root).ByID[5][0]
	if leaf.Has != nil || leaf.Secret != "allowed" {
		t.Fatal("metadata-free native map decoding changed")
	}
}

type metadataRawSpy struct{ raw string }

func (d *metadataRawSpy) Decode(raw []byte, target any) error { return json.Unmarshal(raw, target) }
func (d *metadataRawSpy) UnmarshalWithRepresentation(raw []byte, target any, metadata *shape.Representation) error {
	d.raw = string(raw)
	return structjson.UnmarshalStandard(raw, target, structjson.WithRepresentation(metadata))
}
func TestRepresentationFiltersHiddenAndRetiredNamesBeforeConfiguredDecoder(t *testing.T) {
	metadata := leafMetadata(t, reflect.TypeFor[*metadataLeaf]())
	spy := &metadataRawSpy{}
	decoders := NewDecoderRegistry()
	decoders.Register("application/json", spy)
	source, err := New([]byte(`{"amount":0,"value":99,"secret":"blocked","unknown":"kept"}`), "application/json", nil, WithRepresentation(metadata), WithDecoders(decoders))
	if err != nil {
		t.Fatal(err)
	}
	result, _, err := source.Value(t.Context(), metadata.Original(), "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(spy.raw, `"secret"`) || strings.Contains(spy.raw, `"value"`) || !strings.Contains(spy.raw, `"unknown"`) {
		t.Fatalf("raw admission changed incorrectly: %s", spy.raw)
	}
	actual := result.(*metadataLeaf)
	if actual.Value != 0 || actual.Secret != "" || actual.Has == nil || !actual.Has.Value || actual.Has.Secret {
		t.Fatal("canonical decoded state differs")
	}
}
