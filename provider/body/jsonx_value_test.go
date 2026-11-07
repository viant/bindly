package body

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
)

type convertedValues struct {
	Flag    *bool                                                `json:"flag" jsonx:"intToBool"`
	Catalog *string                                              `json:"catalog" jsonx:"numberToString"`
	Vendors *[]string                                            `json:"vendors" jsonx:"commaStringToSlice"`
	Bill    *string                                              `json:"bill" jsonx:"boolIntToYN,null=omit"`
	Secret  *string                                              `json:"secret" internal:"true" jsonx:"numberToString"`
	Has     *struct{ Flag, Catalog, Vendors, Bill, Secret bool } `json:"-" setMarker:"true"`
}

func decodeValues(t *testing.T, raw string) (convertedValues, error) {
	t.Helper()
	s, err := New([]byte(raw), "application/json", nil)
	if err != nil {
		return convertedValues{}, err
	}
	v, _, err := s.Value(context.Background(), reflect.TypeFor[convertedValues](), "")
	if err != nil {
		return convertedValues{}, err
	}
	return v.(convertedValues), nil
}
func TestJSONXValueConversions(t *testing.T) {
	got, err := decodeValues(t, `{"flag":1,"catalog":9007199254740993,"vendors":" A, B ,,C ","bill":-2,"secret":{},"Has":{"Secret":true}}`)
	if err != nil {
		t.Fatal(err)
	}
	if got.Flag == nil || !*got.Flag || got.Catalog == nil || *got.Catalog != "9007199254740993" || got.Bill == nil || *got.Bill != "Y" || got.Vendors == nil || !reflect.DeepEqual(*got.Vendors, []string{"A", "B", "C"}) {
		t.Fatalf("wrong values: %+v", got)
	}
	if got.Has == nil || !got.Has.Flag || !got.Has.Catalog || !got.Has.Vendors || !got.Has.Bill || got.Has.Secret || got.Secret != nil {
		t.Fatalf("wrong presence: %+v", got.Has)
	}
	raw, _ := json.Marshal(got)
	var output map[string]any
	if err = json.Unmarshal(raw, &output); err != nil {
		t.Fatal(err)
	}
	if output["flag"] != true || output["catalog"] != "9007199254740993" || output["bill"] != "Y" {
		t.Fatalf("output changed: %s", raw)
	}
}
func TestJSONXValuePresenceAndDuplicateOrder(t *testing.T) {
	for _, tc := range []struct {
		raw                                     string
		vendorsPresent, vendorsNil, billPresent bool
	}{
		{`{}`, false, true, false}, {`{"vendors":null,"bill":null}`, true, true, false},
		{`{"vendors":" , "}`, true, true, false}, {`{"vendors":[]}`, true, false, false},
		{`{"bill":true,"bill":null}`, false, true, true},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			got, err := decodeValues(t, tc.raw)
			if err != nil {
				t.Fatal(err)
			}
			if got.Has.Vendors != tc.vendorsPresent || (got.Vendors == nil) != tc.vendorsNil || got.Has.Bill != tc.billPresent {
				t.Fatalf("values=%+v presence=%+v", got, got.Has)
			}
		})
	}
	for _, raw := range []string{`{"flag":2,"flag":1}`, `{"bill":{},"bill":true}`, `{"vendors":[1],"vendors":"ok"}`, `{"catalog":true,"catalog":1}`} {
		if _, err := decodeValues(t, raw); err == nil {
			t.Fatalf("earlier malformed occurrence accepted: %s", raw)
		}
	}
}
func TestJSONXValueNestedAliasSelection(t *testing.T) {
	type row struct {
		Value *string               `json:"value" format:"inputNames=old,inputNamesPriority=canonical" jsonx:"numberToString"`
		Has   *struct{ Value bool } `json:"-" setMarker:"true"`
	}
	type envelope struct {
		Rows []*row          `json:"rows"`
		Map  map[string]*row `json:"map"`
	}
	s, _ := New([]byte(`{"rows":[{"old":{},"value":1.50},null],"map":{"a":{"value":-0,"old":false}}}`), "application/json", nil)
	v, _, err := s.Value(context.Background(), reflect.TypeFor[envelope](), "")
	if err != nil {
		t.Fatal(err)
	}
	got := v.(envelope)
	if *got.Rows[0].Value != "1.50" || !got.Rows[0].Has.Value || got.Rows[1] != nil || *got.Map["a"].Value != "-0" {
		t.Fatalf("wrong nested result: %+v", got)
	}
}
