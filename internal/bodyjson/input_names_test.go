package bodyjson

import (
	"reflect"
	"testing"
)

func TestInputNamesNestedPublicDecodeAndPresence(t *testing.T) {
	type selection struct {
		Metric bool                           `json:"metric" format:"inputNames=metric_alias"`
		Secret bool                           `json:"secret" internal:"true" format:"inputNames=secret_alias"`
		Has    *struct{ Metric, Secret bool } `json:"-" setMarker:"true"`
	}
	type request struct {
		Measures selection `json:"measures"`
	}
	value, err := DecodePublic([]byte(`{"measures":{"metric_alias":true,"secret_alias":true}}`), reflect.TypeFor[request]())
	if err != nil {
		t.Fatal(err)
	}
	got := value.(request).Measures
	if !got.Metric || got.Secret || got.Has == nil || !got.Has.Metric || got.Has.Secret {
		t.Fatalf("alias decode/filter/presence mismatch: %+v marker=%+v", got, got.Has)
	}
}

func TestInputNamesFilterDeniedAliases(t *testing.T) {
	type request struct {
		Secret bool `json:"secret" internal:"true" format:"inputNames=secret_alias"`
	}
	filtered, err := FilterPublic([]byte(`{"secret_alias":true}`), reflect.TypeFor[request]())
	if err != nil {
		t.Fatal(err)
	}
	if string(filtered) != `{}` {
		t.Fatalf("denied alias survived public filter: %s", filtered)
	}
}

func TestInputNamesPresenceUsesCanonicalIdentity(t *testing.T) {
	type selection struct {
		Metric bool                   `json:"metric" format:"inputNames=metric_alias"`
		Has    *struct{ Metric bool } `json:"-" setMarker:"true"`
	}
	for _, raw := range []string{`{"metric_alias":false}`, `{"metric_alias":null}`, `{"metric":true,"metric_alias":false}`} {
		var value selection
		if err := MarkPresence(reflect.ValueOf(&value).Elem(), []byte(raw), true); err != nil {
			t.Fatal(err)
		}
		if value.Has == nil || !value.Has.Metric {
			t.Fatalf("alias not marked present: %s", raw)
		}
	}
	var value selection
	if err := MarkPresence(reflect.ValueOf(&value).Elem(), []byte(`{"METRIC_ALIAS":true}`), true); err != nil {
		t.Fatal(err)
	}
	if value.Has != nil && value.Has.Metric {
		t.Fatal("exact alias incorrectly case folded")
	}
}
