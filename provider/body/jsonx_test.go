package body

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

type jsonxRecord struct {
	Flag   *int                                `json:"flag" jsonx:"boolToInt,null=omit"`
	Secret *int                                `json:"secret" internal:"true" jsonx:"boolToInt,null=omit"`
	Plain  *int                                `json:"plain"`
	Has    *struct{ Flag, Secret, Plain bool } `json:"-" setMarker:"true"`
}

func TestJSONXNativeInput(t *testing.T) {
	type useCase struct {
		raw              string
		value            int
		present, failure bool
	}
	for _, tc := range []useCase{
		{`{}`, 0, false, false}, {`{"flag":null}`, 0, false, false},
		{`{"flag":false}`, 0, true, false}, {`{"flag":true}`, 1, true, false},
		{`{"flag":0}`, 0, true, false}, {`{"flag":1}`, 1, true, false},
		{`{"flag":2}`, 2, true, false}, {`{"flag":-1}`, -1, true, false},
		{`{"FLAG":true,"secret":true,"Has":{"Secret":true}}`, 1, true, false},
		{`{"flag":"0"}`, 0, false, true}, {`{"flag":1.5}`, 0, false, true},
		{`{"flag":[]}`, 0, false, true}, {`{"flag":{}}`, 0, false, true},
		{`{"flag":999999999999999999999999}`, 0, false, true}, {`{"plain":true}`, 0, false, true},
		{`{"flag":true,"flag":null}`, 1, true, false}, {`{"flag":true,"flag":false}`, 0, true, false},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			s, _ := New([]byte(tc.raw), "application/json", nil)
			v, found, err := s.Value(context.Background(), reflect.TypeFor[jsonxRecord](), "")
			if tc.failure {
				if err == nil {
					t.Fatal("expected native type error")
				}
				return
			}
			if err != nil || !found {
				t.Fatalf("decode found=%v err=%v", found, err)
			}
			r := v.(jsonxRecord)
			if (r.Flag != nil) != tc.present || r.Flag != nil && *r.Flag != tc.value {
				t.Fatalf("wrong flag: %+v", r)
			}
			if r.Has == nil || r.Has.Flag != tc.present || r.Has.Secret || r.Secret != nil {
				t.Fatalf("wrong presence: %+v", r.Has)
			}
		})
	}
}

func TestJSONXNamedNestedAndReplay(t *testing.T) {
	type envelope struct {
		Items []*jsonxRecord `json:"items"`
		Child *jsonxRecord   `json:"child"`
	}
	raw := []byte(`{"data":{"items":[{"flag":true},null,{"flag":null},{"flag":-1}],"child":{"flag":false},"unknown":{"flag":true}}}`)
	s, _ := New(raw, "application/json", nil)
	for i := 0; i < 2; i++ {
		value, found, err := s.Value(context.Background(), reflect.TypeFor[envelope](), "data")
		if err != nil || !found {
			t.Fatalf("decode: %v", err)
		}
		got := value.(envelope)
		if len(got.Items) != 4 || *got.Items[0].Flag != 1 || got.Items[1] != nil || got.Items[2].Flag != nil || got.Items[2].Has.Flag || *got.Items[3].Flag != -1 || *got.Child.Flag != 0 || !got.Child.Has.Flag {
			t.Fatalf("nested result: %+v", got)
		}
	}
	captured, found, err := s.CaptureSource(context.Background(), reflect.TypeFor[envelope](), "data")
	if err != nil || !found || !strings.Contains(string(captured.(json.RawMessage)), `"flag":true`) {
		t.Fatalf("raw replay changed: %v", err)
	}
}

func TestJSONXLeavesConfiguredDecoderOwner(t *testing.T) {
	registry := NewDecoderRegistry()
	var received string
	registry.Register("application/json", DecoderFunc(func(raw []byte, target any) error { received = string(raw); return json.Unmarshal(raw, target) }))
	source, _ := New([]byte(`{"flag":true,"secret":true}`), "application/json", nil, WithDecoders(registry))
	_, _, err := source.Value(context.Background(), reflect.TypeFor[jsonxRecord](), "")
	if err == nil || !strings.Contains(received, `"flag":true`) || strings.Contains(received, "secret") {
		t.Fatalf("configured ownership changed: %s %v", received, err)
	}
}

func TestJSONXDateCompositionAndUntaggedNull(t *testing.T) {
	type record struct {
		Flag  *int                              `json:"flag" jsonx:"boolToInt,null=omit"`
		Plain *int                              `json:"plain"`
		Date  time.Time                         `json:"date"`
		Has   *struct{ Flag, Plain, Date bool } `json:"-" setMarker:"true"`
	}
	source, _ := New([]byte(`{"flag":true,"plain":null,"date":"2026-01-02T03:04:05Z"}`), "application/json", nil)
	value, _, err := source.Value(context.Background(), reflect.TypeFor[record](), "")
	if err != nil {
		t.Fatal(err)
	}
	r := value.(record)
	if *r.Flag != 1 || r.Plain != nil || !r.Has.Plain || !r.Has.Date || r.Date.Year() != 2026 {
		t.Fatalf("composition changed: %+v", r)
	}
}
