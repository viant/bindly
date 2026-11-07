package request

import (
	"context"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/viant/bindly"
)

func TestEmptyIntegerQueryPresence(t *testing.T) {
	type marker struct{ Q bool }
	type input struct {
		Q   *int    `bind:"kind=query,in=q"`
		Has *marker `setMarker:"true"`
	}
	for _, tc := range []struct {
		name    string
		query   url.Values
		ignore  bool
		present bool
		want    int
		invalid bool
	}{
		{name: "empty is present zero", query: url.Values{"q": {""}}, present: true},
		{name: "absent stays absent", query: url.Values{}},
		{name: "ignore policy wins", query: url.Values{"q": {""}}, ignore: true},
		{name: "zero", query: url.Values{"q": {"0"}}, present: true},
		{name: "negative", query: url.Values{"q": {"-1"}}, present: true, want: -1},
		{name: "malformed", query: url.Values{"q": {"abc"}}, invalid: true},
		{name: "whitespace", query: url.Values{"q": {" "}}, invalid: true},
		{name: "overflow", query: url.Values{"q": {"999999999999999999999999999999"}}, invalid: true},
		{name: "repeated uses first", query: url.Values{"q": {"7", ""}}, present: true, want: 7},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scope := NewValues(WithQuery(tc.query), WithIgnoreEmptyQueryParameters(tc.ignore))
			inj, err := bindly.NewInjector(bindly.WithProviders(scope.Providers()...))
			if err != nil {
				t.Fatal(err)
			}
			var got input
			err = inj.Bind(context.Background(), &got)
			if tc.invalid {
				if err == nil {
					t.Fatal("expected invalid integer error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			present := got.Has != nil && got.Has.Q
			if present != tc.present || (got.Q != nil) != tc.present {
				t.Fatalf("presence=%t Q=%v, want %t", present, got.Q, tc.present)
			}
			if tc.present && *got.Q != tc.want {
				t.Fatalf("got %d want %d", *got.Q, tc.want)
			}
		})
	}
}

func TestEmptyIntegerPathRemainsInvalid(t *testing.T) {
	scope := NewValues(WithPathParams(map[string]string{"q": ""}))
	inj, err := bindly.NewInjector(bindly.WithProviders(scope.Providers()...))
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Q int `bind:"kind=path,in=q"`
	}
	if err = inj.Bind(context.Background(), &got); err == nil {
		t.Fatal("empty path integer must remain invalid")
	}
}

func TestEmptyIntegerQueryTypesAndBoundaries(t *testing.T) {
	type namedInt int64
	scope := NewValues(WithQuery(url.Values{"q": {""}}))
	for _, sample := range []any{int(0), int8(0), int16(0), int32(0), int64(0), uint(0), uint8(0), uint16(0), uint32(0), uint64(0), namedInt(0)} {
		target := reflect.TypeOf(sample)
		value, found, err := scope.Query().Locate(nil).Value(context.Background(), target, "q")
		if err != nil || !found || !reflect.DeepEqual(value, sample) {
			t.Errorf("target=%v value=%#v found=%t error=%v", target, value, found, err)
		}
	}
	for _, sample := range []any{"", false, float64(0)} {
		value, found, err := scope.Query().Locate(nil).Value(context.Background(), reflect.TypeOf(sample), "q")
		if err != nil || !found || value != "" {
			t.Errorf("noninteger value was converted: %#v %t %v", value, found, err)
		}
	}
	var got struct {
		Q []int `bind:"kind=query,in=q"`
	}
	inj, err := bindly.NewInjector(bindly.WithProviders(scope.Providers()...))
	if err != nil {
		t.Fatal(err)
	}
	if err = inj.Bind(context.Background(), &got); err == nil {
		t.Fatal("empty collection element must remain invalid")
	}
}

func TestEmptyIntegerBodyRemainsInvalid(t *testing.T) {
	req, err := http.NewRequest("POST", "http://example.test", strings.NewReader(`{"q":""}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	scope, err := New(req)
	if err != nil {
		t.Fatal(err)
	}
	inj, err := bindly.NewInjector(bindly.WithProviders(scope.Providers()...))
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Q int `bind:"kind=body,in=q"`
	}
	if err = inj.Bind(context.Background(), &got); err == nil {
		t.Fatal("empty body integer must remain invalid")
	}
}
