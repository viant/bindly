package body

import (
	"reflect"
	"testing"
)

type fallbackRow struct {
	Mode   *string                              `json:"mode" choice:"0:ASAP" choice:"1:SPEND_EVENLY" enumAccept:"scalar" enumStyle:"literal" enumCase:"fold" enumUnknownString:"0" enumUnknownInteger:"0"`
	Budget *string                              `json:"budget" choice:"1:FIXED_BUDGET" choice:"2:AUTO_BUDGET" enumAccept:"scalar" enumCase:"upper" enumUnknownString:"1" enumUnknownInteger:"1"`
	Secret *string                              `json:"secret" internal:"true"`
	Has    *struct{ Mode, Budget, Secret bool } `json:"-" setMarker:"true"`
}

func TestScalarFallbackNativeBody(t *testing.T) {
	type envelope struct {
		Rows []*fallbackRow `json:"rows"`
	}
	for _, tc := range []struct {
		member, want string
		present      bool
	}{
		{``, "", false}, {`,"mode":null`, "", true}, {`,"mode":1`, "SPEND_EVENLY", true},
		{`,"mode":99`, "ASAP", true}, {`,"mode":"Even"`, "ASAP", true}, {`,"mode":"1"`, "ASAP", true},
		{`,"mode":"sPeNd_EvEnLy"`, "SPEND_EVENLY", true}, {`,"mode":-9223372036854775808`, "ASAP", true},
	} {
		raw := []byte(`{"data":{"rows":[{"budget":"auto_budget","secret":"forged","Has":{"Secret":true}` + tc.member + `},null]}}`)
		source, _ := New(raw, "application/json", nil)
		v, _, err := source.Value(t.Context(), reflect.TypeFor[envelope](), "data")
		if err != nil {
			t.Fatal(err)
		}
		rows := v.(envelope).Rows
		row := rows[0]
		if rows[1] != nil || row.Has == nil || row.Has.Mode != tc.present || row.Has.Secret || row.Secret != nil || row.Budget == nil || *row.Budget != "AUTO_BUDGET" || !row.Has.Budget || (row.Mode != nil) != (tc.want != "") || row.Mode != nil && *row.Mode != tc.want {
			t.Fatalf("%s => %+v", tc.member, row)
		}
	}
	for _, raw := range []string{`{"mode":9223372036854775808}`, `{"mode":-9223372036854775809}`, `{"mode":1.0}`, `{"mode":1e0}`, `{"mode":true}`, `{"mode":true,"mode":1}`} {
		source, _ := New([]byte(raw), "application/json", nil)
		if _, _, err := source.Value(t.Context(), reflect.TypeFor[fallbackRow](), ""); err == nil {
			t.Errorf("accepted %s", raw)
		}
	}
}

func TestScalarOrdinalFallbackNativeBody(t *testing.T) {
	type flight struct {
		Mode *int                 `json:"dailyBudgetMode" choice:"1:FIXED_BUDGET" choice:"2:AUTO_BUDGET" enumAccept:"scalar" enumCase:"upper" enumUnknownString:"1" enumUnknownInteger:"1"`
		Has  *struct{ Mode bool } `json:"-" setMarker:"true"`
	}
	for _, tc := range []struct {
		raw     string
		want    int
		present bool
	}{
		{`{}`, 0, false}, {`{"dailyBudgetMode":null}`, 0, true}, {`{"dailyBudgetMode":2}`, 2, true},
		{`{"dailyBudgetMode":99}`, 1, true}, {`{"dailyBudgetMode":-1}`, 1, true}, {`{"dailyBudgetMode":"2"}`, 1, true},
		{`{"dailyBudgetMode":"auto_budget"}`, 2, true}, {`{"dailyBudgetMode":9223372036854775807}`, 1, true},
	} {
		source, _ := New([]byte(tc.raw), "application/json", nil)
		v, _, err := source.Value(t.Context(), reflect.TypeFor[flight](), "")
		if err != nil {
			t.Fatal(err)
		}
		f := v.(flight)
		if (f.Mode != nil) != (tc.want != 0) || f.Mode != nil && *f.Mode != tc.want || (f.Has != nil && f.Has.Mode) != tc.present {
			t.Fatalf("%s => %+v", tc.raw, f)
		}
	}
	for _, raw := range []string{`9223372036854775808`, `-9223372036854775809`, `1.0`, `1e0`, `true`} {
		source, _ := New([]byte(`{"dailyBudgetMode":`+raw+`}`), "application/json", nil)
		if _, _, err := source.Value(t.Context(), reflect.TypeFor[flight](), ""); err == nil {
			t.Errorf("accepted %s", raw)
		}
	}
}
