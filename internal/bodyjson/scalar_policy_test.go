package bodyjson

import (
	"reflect"
	"testing"
)

type openPolicyMode int64
type openPolicyModeRow struct {
	Mode   *openPolicyMode              `json:"mode" choice:"1:FIXED_BUDGET" choice:"2:AUTO_BUDGET" enumAccept:"scalar" enumStyle:"ordinal" enumInteger:"passthrough" enumUnknownString:"0" enumCase:"upper"`
	Hidden *openPolicyMode              `json:"hidden" internal:"true" choice:"1:FIXED_BUDGET" enumAccept:"scalar" enumInteger:"passthrough" enumUnknownString:"0"`
	Has    *struct{ Mode, Hidden bool } `json:"-" setMarker:"true"`
}

func TestOpenScalarPublicBody(t *testing.T) {
	for _, tc := range []struct {
		raw               string
		want              int64
		nilValue, present bool
	}{
		{`{}`, 0, true, false}, {`{"mode":null}`, 0, true, true}, {`{"mode":0}`, 0, false, true}, {`{"mode":9223372036854775807}`, 9223372036854775807, false, true}, {`{"mode":"fıxed_budget"}`, 1, false, true}, {`{"mode":"1"}`, 0, false, true}, {`{"mode":"other"}`, 0, false, true}, {`{"hidden":true}`, 0, true, false},
	} {
		value, err := DecodePublic([]byte(tc.raw), reflect.TypeFor[openPolicyModeRow]())
		if err != nil {
			t.Fatal(err)
		}
		row := value.(openPolicyModeRow)
		if (row.Mode == nil) != tc.nilValue || row.Mode != nil && int64(*row.Mode) != tc.want || (row.Has != nil && row.Has.Mode) != tc.present || row.Hidden != nil {
			t.Fatalf("raw=%s got=%+v", tc.raw, row)
		}
	}
	for _, raw := range []string{`{"mode":1.0}`, `{"mode":1e0}`, `{"mode":true}`, `{"mode":9223372036854775808}`, `{"mode":true,"mode":1}`} {
		if _, err := DecodePublic([]byte(raw), reflect.TypeFor[openPolicyModeRow]()); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
}
