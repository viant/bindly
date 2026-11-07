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

func TestInputAliasPriorityBody(t *testing.T) {
	type row struct {
		ID  *int               `json:"flightId" format:"inputNames=CampaignFlightId,inputNamesPriority=alias,inputNamesCase=fold"`
		Has *struct{ ID bool } `json:"-" setMarker:"true"`
	}
	type envelope struct {
		Rows []row `json:"rows"`
	}
	for _, tc := range []struct {
		raw  string
		want int
		fail bool
	}{
		{`{"CampaignFlightId":2,"flightId":1}`, 2, false}, {`{"flightId":1,"campaignflightid":2}`, 2, false},
		{`{"CampaignFlightId":null,"flightId":1}`, -1, false}, {`{"flightId":"discarded","CampaignFlightId":2}`, 2, false},
		{`{"CampaignFlightId":"bad","flightId":1}`, -1, true}, {`{"CampaignFlightId":2,"flightId":[}`, -1, true},
		{`{"CampaignFlightId":2,"flightId":1,"CAMPAIGNFLIGHTID":3}`, 3, false},
	} {
		raw := `{"rows":[` + tc.raw + `]}`
		s, _ := New([]byte(raw), "application/json", nil)
		v, _, err := s.Value(t.Context(), reflect.TypeFor[envelope](), "")
		if tc.fail {
			if err == nil {
				t.Errorf("accepted %s", raw)
			}
			continue
		}
		if err != nil {
			t.Fatal(raw, err)
		}
		r := v.(envelope).Rows[0]
		got := -1
		if r.ID != nil {
			got = *r.ID
		}
		if got != tc.want || r.Has == nil || !r.Has.ID {
			t.Fatalf("%s got%v has%+v", raw, got, r.Has)
		}
	}
}
