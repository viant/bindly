package body

import (
	"context"
	"reflect"
	"testing"
)

type groupDay struct {
	Canonical *int16                                  `json:"day_of_week" format:"inputGroup=weekday,inputPriority=0,inputNames=dayOfWeek,inputNamesPriority=alias"`
	Upper     *int                                    `json:"Weekday" format:"inputGroup=weekday,inputPriority=1,inputCase=exact"`
	Lower     *int16                                  `json:"weekday" format:"inputGroup=weekday,inputPriority=2,inputCase=exact"`
	Has       *struct{ Canonical, Upper, Lower bool } `json:"-" setMarker:"true"`
}

func TestInputGroupBody(t *testing.T) {
	for _, standard := range []bool{false} {
		for _, tc := range []struct {
			raw   string
			owner int
			fail  bool
		}{
			{`{}`, 0, false}, {`{"WEEKDAY":2}`, 0, false}, {`{"Weekday":2}`, 1, false}, {`{"weekday":2}`, 2, false},
			{`{"Weekday":"bad","weekday":null}`, 2, false}, {`{"weekday":null,"Weekday":"bad"}`, 2, false},
			{`{"day_of_week":"bad","Weekday":32768}`, 1, false}, {`{"Weekday":32768,"day_of_week":"bad"}`, 1, false},
			{`{"weekday":32768}`, 0, true}, {`{"Weekday":"bad","Weekday":2}`, 0, true},
			{`{"day_of_week":1}`, 3, false}, {`{"day_of_week":"bad","dayOfWeek":2}`, 3, false},
		} {
			var got groupDay
			var err error
			source, _ := New([]byte(tc.raw), "application/json", nil)
			var value any
			value, _, err = source.Value(context.Background(), reflect.TypeFor[groupDay](), "")
			if err == nil {
				got = value.(groupDay)
			}
			if tc.fail {
				if err == nil {
					t.Errorf("accepted standard=%v %s", standard, tc.raw)
				}
				continue
			}
			if err != nil {
				t.Errorf("standard=%v %s: %v", standard, tc.raw, err)
				continue
			}
			upper, lower, canonical := false, false, false
			if got.Has != nil {
				upper, lower, canonical = got.Has.Upper, got.Has.Lower, got.Has.Canonical
			}
			if upper != (tc.owner == 1) || lower != (tc.owner == 2) || canonical != (tc.owner == 3) {
				t.Errorf("standard=%v %s Has=%+v", standard, tc.raw, got.Has)
			}
			if tc.owner != 1 && got.Upper != nil || tc.owner != 2 && got.Lower != nil || tc.owner != 3 && got.Canonical != nil {
				t.Errorf("losing value assigned: %+v", got)
			}
		}
	}
}

func TestInputGroupNestedAndHidden(t *testing.T) {
	type row struct {
		Value       *int                                                  `json:"value" format:"inputGroup=x,inputPriority=0"`
		Preferred   *int                                                  `json:"preferred" format:"inputGroup=x,inputPriority=1"`
		Hidden      *int                                                  `json:"hidden" internal:"true" format:"inputGroup=x,inputPriority=100"`
		Independent *int                                                  `json:"independent" format:"inputGroup=y,inputPriority=0"`
		Has         *struct{ Value, Preferred, Hidden, Independent bool } `json:"-" setMarker:"true"`
	}
	type envelope struct {
		Rows []*row          `json:"rows"`
		Map  map[string]*row `json:"map"`
	}
	raw := `{"rows":[{"value":"bad","preferred":2,"hidden":{},"independent":3},null],"map":{"a":{"value":"bad","preferred":null}}}`
	s, _ := New([]byte(raw), "application/json", nil)
	v, _, err := s.Value(context.Background(), reflect.TypeFor[envelope](), "")
	if err != nil {
		t.Fatal(err)
	}
	got := v.(envelope)
	if got.Rows[0].Value != nil || *got.Rows[0].Preferred != 2 || got.Rows[0].Hidden != nil || *got.Rows[0].Independent != 3 || got.Rows[1] != nil {
		t.Fatalf("wrong values %+v", got)
	}
	h := got.Rows[0].Has
	if h == nil || h.Value || !h.Preferred || h.Hidden || !h.Independent {
		t.Fatalf("wrong presence %+v", h)
	}
	if got.Map["a"].Has == nil || !got.Map["a"].Has.Preferred || got.Map["a"].Has.Value {
		t.Fatalf("wrong map presence %+v", got.Map["a"].Has)
	}
}

func TestInputCanonicalVariantExclusion(t *testing.T) {
	type row struct {
		Status int                    `json:"Status" format:"inputNames=status,inputNamesExclude=status"`
		Has    *struct{ Status bool } `json:"-" setMarker:"true"`
	}
	for _, tc := range []struct {
		raw     string
		want    int
		present bool
	}{{`{"status":"ignored"}`, 0, false}, {`{"Status":2,"status":{}}`, 2, true}, {`{"status":{},"STATUS":3}`, 3, true}} {
		s, _ := New([]byte(tc.raw), "application/json", nil)
		v, _, err := s.Value(context.Background(), reflect.TypeFor[row](), "")
		if err != nil {
			t.Fatal(tc.raw, err)
		}
		got := v.(row)
		if got.Status != tc.want || got.Has.Status != tc.present {
			t.Fatalf("%s got%+v has%+v", tc.raw, got, got.Has)
		}
	}
}
