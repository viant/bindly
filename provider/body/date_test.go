package body

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

type dateRecord struct {
	When   *time.Time                          `json:"when"`
	Child  *dateRecord                         `json:"child"`
	Secret *time.Time                          `internal:"true"`
	Has    *struct{ When, Child, Secret bool } `json:"-" setMarker:"true"`
}

func TestDefaultDatesBodyNullPolicyAndNativePresence(t *testing.T) {
	for _, policy := range []string{"", "empty-record"} {
		for _, raw := range []string{"", "null", "{}", `{"when":"2026-10-11 17:28:39","child":null,"Secret":123}`, `{"when":null}`, `{"when":"0001-01-01T00:00:00Z"}`} {
			t.Run(policy+":"+raw, func(t *testing.T) {
				source, _ := New([]byte(raw), "application/json", nil)
				value, found, err := source.ValueWithBodyNullPolicy(context.Background(), reflect.TypeFor[*dateRecord](), "", policy)
				if err != nil {
					t.Fatal(err)
				}
				if raw == "" {
					if found {
						t.Fatal("missing became present")
					}
					return
				}
				if !found {
					t.Fatal("body lost presence")
				}
				if raw == "null" && policy == "" {
					if value != nil {
						t.Fatal("ordinary whole null changed")
					}
					return
				}
				v := value.(*dateRecord)
				if v == nil || v.Has == nil || v.Secret != nil || v.Has.Secret {
					t.Fatalf("record/markers changed %+v", v)
				}
				if v.Has.When != strings.Contains(raw, `"when"`) || v.Has.Child != strings.Contains(raw, `"child"`) {
					t.Fatalf("presence %+v", v.Has)
				}
				if strings.Contains(raw, "2026-10-11") && (v.When == nil || v.When.Format(time.DateOnly) != "2026-10-11") {
					t.Fatal("nonnull policy bypassed dates")
				}
				if strings.Contains(raw, "0001") && v.When != nil {
					t.Fatal("fresh parsed zero pointer populated")
				}
			})
		}
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	source, _ := New([]byte(`{"when":"2026-10-11"}`), "application/json", nil)
	if _, _, err := source.ValueWithBodyNullPolicy(cancelled, reflect.TypeFor[*dateRecord](), "", "empty-record"); err == nil {
		t.Fatal("cancelled policy accepted")
	}
}
func TestDefaultDatesRegisteredDecoderOwnsOriginalDateBytes(t *testing.T) {
	calls := 0
	registry := NewDecoderRegistry()
	registry.Register("application/json", DecoderFunc(func(raw []byte, target any) error {
		calls++
		if strings.Contains(string(raw), "Secret") || !strings.Contains(string(raw), "2026-10-11 17:28:39") {
			t.Fatalf("registered owner raw changed %s", raw)
		}
		// The owner, not the native default, chooses its protocol conversion.
		encoded := strings.ReplaceAll(string(raw), "2026-10-11 17:28:39", "2026-10-11T17:28:39Z")
		return json.Unmarshal([]byte(encoded), target)
	}))
	source, _ := New([]byte(`{"when":"2026-10-11 17:28:39","Secret":false}`), "application/json", nil, WithDecoders(registry))
	value, _, err := source.Value(context.Background(), reflect.TypeFor[*dateRecord](), "")
	if err != nil || calls != 1 || value.(*dateRecord).When == nil || !value.(*dateRecord).Has.When {
		t.Fatalf("registered decoder displaced: calls=%d err=%v", calls, err)
	}
}

func TestDefaultDatesRegisteredDecoderDoesNotValidateNativeDateMetadata(t *testing.T) {
	type record struct {
		When   *time.Time                   `json:"when" format:"timezone=Definitely/Invalid"`
		Secret *time.Time                   `internal:"true"`
		Has    *struct{ When, Secret bool } `json:"-" setMarker:"true"`
	}
	type nested struct {
		Child *record               `json:"child"`
		Has   *struct{ Child bool } `json:"-" setMarker:"true"`
	}
	for _, tc := range []struct {
		name, raw string
		target    reflect.Type
	}{
		{"direct", `{"when":"2026-10-11 17:28:39","Secret":false}`, reflect.TypeFor[*record]()},
		{"nested", `{"child":{"when":"2026-10-11 17:28:39","Secret":false}}`, reflect.TypeFor[*nested]()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			registry := NewDecoderRegistry()
			registry.Register("application/json", DecoderFunc(func(raw []byte, target any) error {
				calls++
				if strings.Contains(string(raw), "Secret") || !strings.Contains(string(raw), "2026-10-11 17:28:39") {
					t.Fatalf("registered filtering/date owner changed: %s", raw)
				}
				return json.Unmarshal([]byte(strings.ReplaceAll(string(raw), "2026-10-11 17:28:39", "2026-10-11T17:28:39Z")), target)
			}))
			source, _ := New([]byte(tc.raw), "application/json", nil, WithDecoders(registry))
			value, found, err := source.Value(context.Background(), tc.target, "")
			if err != nil || !found || calls != 1 {
				t.Fatalf("registered owner rejected: calls=%d found=%v err=%v", calls, found, err)
			}
			var actual *record
			switch v := value.(type) {
			case *record:
				actual = v
			case *nested:
				if v.Has == nil || !v.Has.Child {
					t.Fatal("nested presence lost")
				}
				actual = v.Child
			}
			if actual == nil || actual.When == nil || actual.When.Format(time.RFC3339Nano) != "2026-10-11T17:28:39Z" || actual.Secret != nil || actual.Has == nil || !actual.Has.When || actual.Has.Secret {
				t.Fatalf("registered value/filter/presence changed: %+v", actual)
			}
			// Native public decoding retains date metadata validation; only the
			// registered owner and its presence reconstruction are separated.
			native, _ := New([]byte(tc.raw), "application/json", nil)
			_, _, err = native.Value(context.Background(), tc.target, "")
			var classified interface{ StatusCode() int }
			cause := errors.Unwrap(err)
			if err == nil || !errors.As(err, &classified) || classified.StatusCode() != 400 || cause == nil || !strings.Contains(cause.Error(), "invalid timezone") {
				t.Fatalf("native date metadata validation changed: %v", err)
			}
		})
	}
}
