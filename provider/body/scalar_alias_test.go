package body

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
)

type scalarAliasMode string

type scalarAliasRow struct {
	Mode   *scalarAliasMode             `json:"mode" choice:"0:ASAP" choice:"1:SPEND_EVENLY" enumAccept:"scalar" enumStyle:"literal" enumAlias:"0:0" enumAlias:"1:1" enumAlias:"1:Even" enumCase:"fold"`
	Secret string                       `json:"secret" internal:"true"`
	Has    *struct{ Mode, Secret bool } `json:"-" setMarker:"true"`
}

func TestScalarAliasesNativeBodyAndPresence(t *testing.T) {
	type envelope struct {
		Rows []*scalarAliasRow `json:"rows"`
	}
	for _, tc := range []struct {
		name, member, want string
		present, invalid   bool
	}{
		{"omitted", ``, "", false, false},
		{"null", `,"mode":null`, "", true, false},
		{"numeric zero", `,"mode":0`, "ASAP", true, false},
		{"numeric one", `,"mode":1`, "SPEND_EVENLY", true, false},
		{"string zero", `,"mode":"0"`, "ASAP", true, false},
		{"string one", `,"mode":"1"`, "SPEND_EVENLY", true, false},
		{"display alias", `,"mode":"eVeN"`, "SPEND_EVENLY", true, false},
		{"unicode fold", `,"mode":"ſPEND_EVENLY"`, "SPEND_EVENLY", true, false},
		{"unknown", `,"mode":2`, "", false, true},
		{"unknown name", `,"mode":"other"`, "", false, true},
		{"fraction", `,"mode":1.5`, "", false, true},
		{"boolean", `,"mode":true`, "", false, true},
		{"object", `,"mode":{}`, "", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := []byte(`{"data":{"rows":[{"secret":"forged","Has":{"Mode":true,"Secret":true}` + tc.member + `},null]}}`)
			source, err := New(raw, "application/json", nil)
			if err != nil {
				t.Fatal(err)
			}
			value, found, err := source.Value(context.Background(), reflect.TypeFor[envelope](), "data")
			if tc.invalid {
				if err == nil {
					t.Fatal("invalid scalar alias accepted")
				}
				return
			}
			if err != nil || !found {
				t.Fatalf("body found=%v error=%v", found, err)
			}
			rows := value.(envelope).Rows
			if len(rows) != 2 || rows[0] == nil || rows[1] != nil {
				t.Fatalf("nested shape changed: %+v", rows)
			}
			row := rows[0]
			if row.Has == nil || row.Has.Mode != tc.present || row.Has.Secret || row.Secret != "" {
				t.Fatalf("presence/internal filtering changed: %+v", row)
			}
			if tc.want == "" {
				if row.Mode != nil {
					t.Fatalf("expected absent/null pointer, got %v", *row.Mode)
				}
			} else if row.Mode == nil || string(*row.Mode) != tc.want {
				t.Fatalf("mode=%v, want %s", row.Mode, tc.want)
			}
			captured, found, err := source.CaptureSource(context.Background(), reflect.TypeFor[envelope](), "data")
			if err != nil || !found || !json.Valid(captured.(json.RawMessage)) {
				t.Fatalf("capture found=%v error=%v", found, err)
			}
		})
	}
}
