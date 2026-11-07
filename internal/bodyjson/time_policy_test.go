package bodyjson

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

type policyDateRow struct {
	Value  *time.Time                    `json:"value" format:"timeLayout=2006-01-02T15:04:05Z07:00,timeInputLayouts=2006-01-02T15:04:05.999999999Z07:00|2006-01-02T15:04:05|2006-01-02 15:04:05|2006-01-02,timeEpoch=milliseconds,timeEmpty=preserve,timezone=UTC,nullable=true"`
	Hidden *time.Time                    `json:"hidden" internal:"true" format:"timeInputLayouts=2006-01-02,timeEpoch=milliseconds,timeEmpty=preserve"`
	Has    *struct{ Value, Hidden bool } `json:"-" setMarker:"true"`
}

func TestDeclaredTimePolicy(t *testing.T) {
	for _, tc := range []struct {
		name, raw, want string
		present, fail   bool
	}{
		{"missing", `{}`, "nil", false, false}, {"null", `{"value":null}`, "nil", true, false},
		{"empty", `{"value":""}`, "0001-01-01T00:00:00Z", true, false},
		{"epoch", `{"value":0}`, "1970-01-01T00:00:00Z", true, false},
		{"max epoch", `{"value":9223372036854775807}`, time.UnixMilli(9223372036854775807).UTC().Format(time.RFC3339Nano), true, false},
		{"negative", `{"value":-1}`, "1969-12-31T23:59:59.999Z", true, false},
		{"date", `{"value":"2026-11-07"}`, "2026-11-07T00:00:00Z", true, false},
		{"offset", `{"value":"2026-11-07T08:00:00-08:00"}`, "2026-11-07T16:00:00Z", true, false},
		{"duplicate empty", `{"value":"2026-11-07","value":""}`, "2026-11-07T00:00:00Z", true, false},
		{"duplicate null", `{"value":"2026-11-07","value":null}`, "nil", true, false},
		{"null empty", `{"value":null,"value":""}`, "0001-01-01T00:00:00Z", true, false},
		{"case variant", `{"VALUE":"2026-11-07","value":""}`, "2026-11-07T00:00:00Z", true, false},
		{"invalid earlier", `{"value":"bad","value":"2026-11-07"}`, "", false, true},
		{"invalid later", `{"value":"2026-11-07","value":"bad"}`, "", false, true},
		{"exponent", `{"value":1e3}`, "", false, true}, {"fraction", `{"value":1.0}`, "", false, true},
		{"overflow", `{"value":9223372036854775808}`, "", false, true},
		{"internal ignored", `{"hidden":"bad"}`, "nil", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := DecodePublic([]byte(tc.raw), reflect.TypeFor[policyDateRow]())
			if (err != nil) != tc.fail {
				t.Fatalf("error=%v", err)
			}
			if err != nil {
				return
			}
			row := got.(policyDateRow)
			actual := "nil"
			if row.Value != nil {
				actual = row.Value.Format(time.RFC3339Nano)
			}
			if actual != tc.want {
				t.Fatalf("value=%s want %s", actual, tc.want)
			}
			if (row.Has != nil && row.Has.Value) != tc.present {
				t.Fatalf("presence=%+v", row.Has)
			}
			if row.Hidden != nil || row.Has != nil && row.Has.Hidden {
				t.Fatalf("internal exposed")
			}
		})
	}
}

func TestDeclaredTimeNestedBody(t *testing.T) {
	type body struct {
		Rows  []policyDateRow           `json:"rows"`
		Named map[string]*policyDateRow `json:"named"`
	}
	for _, raw := range []string{
		`{"rows":[{"value":0}],"named":{"a":{"value":"2026-11-07","value":""}}}`,
		`{"rows":[{"value":0}],"named":{"a":{"value":"bad","value":"2026-11-07"}}}`,
	} {
		got, err := DecodePublic([]byte(raw), reflect.TypeFor[body]())
		if strings.Contains(raw, "bad") {
			if err == nil {
				t.Fatal("lost earlier invalid duplicate")
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		value := got.(body)
		if value.Rows[0].Value == nil || !value.Rows[0].Value.Equal(time.UnixMilli(0)) || value.Named["a"].Value.Format(time.RFC3339) != "2026-11-07T00:00:00Z" {
			t.Fatalf("wrong value %+v", value)
		}
	}
}
