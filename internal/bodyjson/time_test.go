package bodyjson

import (
	"encoding/json"
	"errors"
	"fmt"
	legacytime "github.com/viant/bindly/internal/bodyjson/testdata/legacytime"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

type dateHas struct{ Value, Pointer bool }
type dateRow struct {
	Value   time.Time  `json:"value"`
	Pointer *time.Time `json:"pointer"`
	Has     *dateHas   `setMarker:"true" json:"-"`
}

// These fixtures were emitted by the actual pinned Datly69de JSON marshaller
// linked with original Taglyeade. See the frozen packet's oracle source/hashes.
func TestDefaultDatesActualLegacyMarshallerOracle(t *testing.T) {
	var cases []struct {
		Raw, Error, Panic, Value string
		Pointer                  *string
		Has                      *dateHas
	}
	raw, err := os.ReadFile("testdata/legacy_dates.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	for i, tc := range cases {
		t.Run(fmt.Sprintf("%02d", i), func(t *testing.T) {
			value, err := DecodePublic([]byte(tc.Raw), reflect.TypeFor[dateRow]())
			if tc.Panic != "" || tc.Error != "" {
				if err == nil {
					t.Fatalf("source reject/panic accepted: %s", tc.Raw)
				}
				var input interface{ StatusCode() int }
				if !errors.As(err, &input) || input.StatusCode() != 400 {
					t.Fatalf("unclassified input error: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("source accepted %s: %v", tc.Raw, err)
			}
			actual := value.(dateRow)
			if actual.Value.Format(time.RFC3339Nano) != tc.Value {
				t.Fatalf("%s value=%s source=%s", tc.Raw, actual.Value.Format(time.RFC3339Nano), tc.Value)
			}
			if (actual.Pointer == nil) != (tc.Pointer == nil) {
				t.Fatalf("%s pointer nilness differs", tc.Raw)
			}
			if actual.Pointer != nil && actual.Pointer.Format(time.RFC3339Nano) != *tc.Pointer {
				t.Fatalf("%s pointer=%s source=%s", tc.Raw, actual.Pointer.Format(time.RFC3339Nano), *tc.Pointer)
			}
			if !reflect.DeepEqual(actual.Has, tc.Has) {
				t.Fatalf("%s presence=%+v source=%+v", tc.Raw, actual.Has, tc.Has)
			}
		})
	}
}
func TestDefaultParserDifferentialOriginalPinnedSource(t *testing.T) {
	values := []string{"", "Z", "nonsense", "2026-10-11", "2026-10-11 17:28:39", "2026-10-11T17:28:39+05:30", "2026-10-11T17:28:39.123456789Z", "2026-10-11T17:28:39ZZ", "0001-01-01T00:00:00Z", "2026-99-11", "2026-10-11+05:30"}
	for _, value := range values {
		trimmed := strings.TrimRight(value, "Z")
		var old time.Time
		var oldErr error
		var panicked bool
		func() {
			defer func() { panicked = recover() != nil }()
			old, oldErr = legacytime.Parse(time.RFC3339, trimmed)
		}()
		actual, err := parseLegacyTime(time.RFC3339, trimmed)
		if panicked {
			if err == nil {
				t.Fatalf("legacy panic was not hardened: %q", value)
			}
			continue
		}
		if (err == nil) != (oldErr == nil) || err == nil && (!actual.Equal(old) || actual.Format(time.RFC3339Nano) != old.Format(time.RFC3339Nano)) {
			t.Fatalf("%q actual=%s/%v source=%s/%v", value, actual, err, old, oldErr)
		}
	}
}

type opaqueDate struct{ Raw string }

func (p *opaqueDate) UnmarshalJSON(raw []byte) error { p.Raw = string(raw); return nil }

type textDate string

func (p *textDate) UnmarshalText(raw []byte) error { *p = textDate(raw); return nil }

type namedDate time.Time

func (p *namedDate) UnmarshalJSON(raw []byte) error { return json.Unmarshal(raw, (*time.Time)(p)) }

type exactAlias = time.Time
type graphDates struct {
	When     *time.Time                                                                                                           `json:"when"`
	Value    time.Time                                                                                                            `json:"value"`
	Children []*graphDates                                                                                                        `json:"children"`
	Next     *graphDates                                                                                                          `json:"next"`
	Array    [2]*time.Time                                                                                                        `json:"array"`
	Map      map[string]*graphDates                                                                                               `json:"map"`
	Dates    map[string]*time.Time                                                                                                `json:"dates"`
	Raw      json.RawMessage                                                                                                      `json:"raw"`
	Any      any                                                                                                                  `json:"any"`
	Custom   *opaqueDate                                                                                                          `json:"custom"`
	Text     textDate                                                                                                             `json:"text"`
	Named    *namedDate                                                                                                           `json:"named"`
	Alias    exactAlias                                                                                                           `json:"alias"`
	Secret   *time.Time                                                                                                           `internal:"true"`
	Hidden   *time.Time                                                                                                           `json:"-"`
	Has      *struct{ When, Value, Children, Next, Array, Map, Dates, Raw, Any, Custom, Text, Named, Alias, Secret, Hidden bool } `json:"-" setMarker:"true"`
}

func TestDefaultDatesCombinedPublicGraphOwnership(t *testing.T) {
	raw := `{"when":"2026-10-11 17:28:39","value":"2026-10-11","children":[{"when":"2026-10-11","next":{"when":"2026-10-12"}},null],"next":{"children":[{"when":"2026-10-13"}]},"array":["2026-10-11",null],"map":{"a":{"when":"2026-10-11"}},"dates":{"a":"2026-10-11"},"raw":{"when":"untouched","n":1,"n":2},"any":{"when":"untouched"},"custom":{"when":"untouched","n":1,"n":2},"text":"untouched","named":"2026-10-11T17:28:39Z","alias":"2026-10-11","Secret":123,"Hidden":"bad","Has":{"When":false,"Secret":true},"unknown":{"when":"bad"}}`
	value, err := DecodePublic([]byte(raw), reflect.TypeFor[*graphDates]())
	if err != nil {
		t.Fatal(err)
	}
	v := value.(*graphDates)
	if v.When == nil || v.Children[0].Next.When == nil || v.Next.Children[0].When == nil || v.Array[0] == nil || v.Array[1] != nil || v.Map["a"].When == nil || v.Dates["a"] == nil || v.Secret != nil || v.Hidden != nil || !v.Has.When || v.Has.Secret || v.Has.Hidden {
		t.Fatalf("graph/presence differs: %+v", v)
	}
	if string(v.Raw) != `{"when":"untouched","n":1,"n":2}` || v.Custom.Raw != `{"when":"untouched","n":1,"n":2}` || v.Text != "untouched" || v.Any.(map[string]any)["when"] != "untouched" {
		t.Fatal("opaque JSON/Text owner changed")
	}
	if _, err := DecodePublic([]byte(`{"named":"2026-10-11 17:28:39"}`), reflect.TypeFor[graphDates]()); err == nil {
		t.Fatal("named custom time owner overridden")
	}
}
func TestDefaultDatesDuplicatesAndNestedPointerReuse(t *testing.T) {
	for _, raw := range []string{
		`{"when":"2026-10-11","WHEN":"0001-01-01T00:00:00Z"}`,
		`{"when":"2026-10-11","when":null}`,
		`{"next":{"when":"2026-10-11"},"NEXT":{"when":"0001-01-01T00:00:00Z"}}`,
		`{"children":[{"when":"2026-10-11"}],"CHILDREN":[{"when":"0001-01-01T00:00:00Z"}]}`,
	} {
		value, err := DecodePublic([]byte(raw), reflect.TypeFor[graphDates]())
		if err != nil {
			t.Fatal(err)
		}
		v := value.(graphDates)
		stamp := v.When
		if v.Next != nil {
			stamp = v.Next.When
		}
		if len(v.Children) > 0 {
			stamp = v.Children[0].When
		}
		if stamp == nil || stamp.Format(time.DateOnly) != "2026-10-11" {
			t.Fatalf("pointer reuse failed %s: %+v", raw, v)
		}
	}
	// Map entries are decoded freshly, even for duplicate map keys.
	value, err := DecodePublic([]byte(`{"dates":{"a":"2026-10-11","a":"0001-01-01T00:00:00Z"}}`), reflect.TypeFor[graphDates]())
	if err != nil || value.(graphDates).Dates["a"] != nil {
		t.Fatalf("map pointer reuse invented: %v", err)
	}
	for _, raw := range []string{`{"when":123,"when":"2026-10-11"}`, `{"when":"2026-10-11"} junk`, `{"when":"2026-10-11",}`, `{"when":false}`} {
		if _, err := DecodePublic([]byte(raw), reflect.TypeFor[graphDates]()); err == nil {
			t.Fatalf("invalid earlier occurrence/syntax suppressed: %s", raw)
		}
	}
}
func TestDefaultDatesFilterAndTypedReplayKeepOwnership(t *testing.T) {
	raw := []byte(`{"when":"2026-10-11 17:28:39","Secret":"bad","custom":{"n":1,"n":2}}`)
	filtered, err := FilterPublic(raw, reflect.TypeFor[graphDates]())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(filtered), "Secret") || !strings.Contains(string(filtered), "2026-10-11 17:28:39") || !strings.Contains(string(filtered), `{"n":1,"n":2}`) {
		t.Fatalf("registered decoder raw changed: %s", filtered)
	}
	zero := time.Time{}
	typed := dateRow{Pointer: &zero}
	encoded, _ := json.Marshal(typed)
	v, err := VerifyTypedRoundtrip(encoded, reflect.TypeFor[dateRow]())
	if err != nil || v.(dateRow).Pointer == nil || !v.(dateRow).Pointer.IsZero() {
		t.Fatalf("typed replay zero pointer changed: %v", err)
	}
	type authored struct {
		When *time.Time `json:"when" format:"timeLayout=2006-01-02 15:04:05"`
	}
	if _, err = DecodePublic([]byte(`{"when":"2026-10-11 17:28:39"}`), reflect.TypeFor[authored]()); err == nil {
		t.Fatal("explicit format owner replaced")
	}
	v, err = DecodePublic([]byte(`{"when":"0001-01-01T00:00:00Z"}`), reflect.TypeFor[authored]())
	if err != nil || v.(authored).When == nil {
		t.Fatalf("authored zero pointer default nilness imposed: %v", err)
	}
}
func TestDefaultDatesConcurrentImmutablePlans(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				v, err := DecodePublic([]byte(`{"when":"2026-10-11","next":{"when":"2026-10-12"}}`), reflect.TypeFor[graphDates]())
				if err != nil || v.(graphDates).Next.When == nil {
					t.Errorf("concurrent normalization: %v", err)
				}
			}
		}()
	}
	wg.Wait()
}
func TestDefaultDatesCompiledFieldDominance(t *testing.T) {
	type first struct {
		When *time.Time `json:"when"`
	}
	type second struct {
		When string `json:"when"`
	}
	type ambiguous struct {
		first
		second
		Other *time.Time `json:"other"`
	}
	v, err := DecodePublic([]byte(`{"when":{"untouched":true},"other":"2026-10-11"}`), reflect.TypeFor[ambiguous]())
	if err != nil || v.(ambiguous).Other == nil {
		t.Fatalf("ambiguous field rewritten: %v", err)
	}
	type dominant struct {
		first
		When  string     `json:"when"`
		Other *time.Time `json:"other"`
	}
	v, err = DecodePublic([]byte(`{"when":"untouched","other":"2026-10-11"}`), reflect.TypeFor[dominant]())
	if err != nil || v.(dominant).When != "untouched" || v.(dominant).first.When != nil {
		t.Fatalf("embedding dominance changed: %v", err)
	}
}

func TestDefaultDatesArrayDiscardedTailIsNotNormalized(t *testing.T) {
	type record struct{ Dates [1]*time.Time }
	value, err := DecodePublic([]byte(`{"Dates":["2026-10-11",false,{"wrong":"type"}]}`), reflect.TypeFor[record]())
	if err != nil || value.(record).Dates[0] == nil {
		t.Fatalf("native discarded array values inspected: %v", err)
	}
}

func TestDefaultDatesEmptySliceResetsDuplicateBacking(t *testing.T) {
	value, err := DecodePublic([]byte(`{"children":[{"when":"2026-10-11"}],"children":[],"children":[{"when":"0001-01-01T00:00:00Z"}]}`), reflect.TypeFor[graphDates]())
	if err != nil {
		t.Fatal(err)
	}
	actual := value.(graphDates)
	if len(actual.Children) != 1 || actual.Children[0].When != nil || !actual.Children[0].Has.When {
		t.Fatal("empty slice retained an earlier duplicate pointer")
	}
}
