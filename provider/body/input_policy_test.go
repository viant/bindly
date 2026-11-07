package body

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

type policyCap struct {
	Count int `json:"count"`
}
type policyRow struct {
	Cap     *policyCap                           `json:"frequencyCapping" format:"inputNames=frequencyCap,inputNamesPriority=canonical,inputNamesKinds=object|null,inputNamesCase=fold"`
	Flights []policyCap                          `json:"campaignFlightCaps" format:"inputNames=flightConstraints,inputNamesPriority=canonical,inputNamesCase=fold,inputNamesExclude=FlightConstraints"`
	Secret  string                               `json:"secret" internal:"true"`
	Has     *struct{ Cap, Flights, Secret bool } `json:"-" setMarker:"true"`
}

func TestInputPolicyNativeBody(t *testing.T) {
	for _, tc := range []struct {
		members string
		want    int
		present bool
	}{
		{`"frequencyCap":3`, -1, false}, {`"frequencyCap":null`, -1, true}, {`"FREQUENCYCAP":{"count":2}`, 2, true},
		{`"frequencyCapping":{"count":1},"frequencyCap":{"count":2}`, 1, true},
		{`"frequencyCap":{"count":2},"frequencyCapping":{"count":1}`, 1, true},
		{`"frequencyCapping":{"count":1},"frequencyCap":null`, 1, true},
		{`"frequencyCap":{"count":"bad"},"frequencyCapping":{"count":1}`, 1, true},
	} {
		type envelope struct {
			Rows []policyRow `json:"rows"`
		}
		raw := []byte(`{"data":{"rows":[{` + tc.members + `,"secret":"forged","Has":{"Secret":true},"flightConstraints":[{"count":2}],"FlightConstraints":[{"count":3}]}]}}`)
		s, _ := New(raw, "application/json", nil)
		v, _, err := s.Value(t.Context(), reflect.TypeFor[envelope](), "data")
		if err != nil {
			t.Fatalf("%s: %v", raw, err)
		}
		r := v.(envelope).Rows[0]
		got := -1
		if r.Cap != nil {
			got = r.Cap.Count
		}
		if got != tc.want || r.Has.Cap != tc.present || r.Secret != "" || r.Has.Secret || len(r.Flights) != 1 || r.Flights[0].Count != 2 || !r.Has.Flights {
			t.Fatalf("%s => %+v Has%+v", raw, r, r.Has)
		}
		captured, _, err := s.CaptureSource(t.Context(), reflect.TypeFor[envelope](), "data")
		if err != nil || !strings.Contains(string(captured.(json.RawMessage)), `"FlightConstraints"`) {
			t.Fatal("capture changed", err)
		}
	}
}
