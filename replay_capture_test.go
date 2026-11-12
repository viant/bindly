package bindly

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/viant/bindly/locator"
	"github.com/viant/bindly/provider/body"
	"github.com/viant/bindly/provider/request"
	"github.com/viant/bindly/provider/values"
	"github.com/viant/bindly/state"
	"github.com/viant/structology"
)

type sourceCaptureBody struct {
	Number   int64                                         `json:"number"`
	Zero     int                                           `json:"zero"`
	Absent   string                                        `json:"absent"`
	Nullable *string                                       `json:"nullable"`
	Has      struct{ Number, Zero, Absent, Nullable bool } `setMarker:"true" json:"-"`
}

func TestReplayCaptureSourcesRawBodyPresence(t *testing.T) {
	for _, name := range []string{"", "entity"} {
		for _, raw := range []string{`{"number":9007199254740993,"zero":0,"nullable":null}`, `{}`, `null`} {
			t.Run(name+raw, func(t *testing.T) {
				type input struct {
					Body *sourceCaptureBody
					Has  struct{ Body bool } `setMarker:"true"`
				}
				injector, err := NewInjector()
				require.NoError(t, err)
				plan, err := injector.CompilePlan(reflect.TypeFor[input](), BindingSpec{Path: "Body", Name: "CanonicalBody", Location: state.Location{Kind: "body", In: name}})
				require.NoError(t, err)
				selected, err := plan.Replay("Body")
				require.NoError(t, err)
				text := raw
				if name != "" {
					text = `{"entity":` + raw + `}`
				}
				src, err := body.New([]byte(text), "application/json", nil)
				require.NoError(t, err)
				// Exercise the normal composed-provider layer as well as the raw body owner.
				provider, err := locator.ComposeProviders("body", locator.ProviderLayer{Name: "request", Provider: src})
				require.NoError(t, err)
				replay, err := selected.CaptureSources(context.Background(), []locator.Provider{provider})
				require.NoError(t, err)
				encoded, err := json.Marshal(replay)
				require.NoError(t, err)
				require.JSONEq(t, `{"CanonicalBody":`+raw+`}`, string(encoded))
				scope, err := injector.ForScope(provider)
				require.NoError(t, err)
				inScope, err := selected.CaptureSourcesIn(t.Context(), scope)
				require.NoError(t, err)
				scopedJSON, err := json.Marshal(inScope)
				require.NoError(t, err)
				require.JSONEq(t, string(encoded), string(scopedJSON))
				actual := &input{}
				require.NoError(t, injector.Bind(context.Background(), actual, WithPlan(plan), WithReplay(ReplayBinding{Replay: replay, Only: true})))
				if raw != "null" {
					require.NotNil(t, actual.Body)
					if raw != "{}" {
						require.EqualValues(t, 9007199254740993, actual.Body.Number)
						require.True(t, actual.Body.Has.Zero)
						require.True(t, actual.Body.Has.Nullable)
					}
					require.False(t, actual.Body.Has.Absent)
				} else {
					require.Nil(t, actual.Body)
					require.True(t, actual.Has.Body)
				}
				prepared, err := json.Marshal(replay)
				require.NoError(t, err)
				require.JSONEq(t, string(encoded), string(prepared))
			})
		}
	}
}
func TestReplayCaptureSourcesMissingDefaultsAndProviderTypes(t *testing.T) {
	type input struct {
		ID      int
		Names   []string
		Token   string
		Empty   string
		Default int
		Current int
	}
	injector, err := NewInjector()
	require.NoError(t, err)
	plan, err := injector.CompilePlan(reflect.TypeFor[input](),
		BindingSpec{Path: "ID", Location: state.Location{Kind: "query", In: "id"}},
		BindingSpec{Path: "Names", Location: state.Location{Kind: "query", In: "name"}},
		BindingSpec{Path: "Token", Location: state.Location{Kind: "header", In: "Authorization"}},
		BindingSpec{Path: "Empty", Location: state.Location{Kind: "query", In: "empty"}},
		BindingSpec{Path: "Default", Location: state.Location{Kind: "query", In: "default"}, DefaultValue: 8},
		BindingSpec{Path: "Current", Location: state.Location{Kind: "dependency"}})
	require.NoError(t, err)
	selected, err := plan.Replay("ID", "Names", "Token", "Empty", "Default")
	require.NoError(t, err)
	req := httptest.NewRequest("POST", "/items?id=7&name=one&name=two&empty=", nil)
	req.Header.Set("Authorization", "signed")
	scope, err := request.New(req)
	require.NoError(t, err)
	defer scope.Close()
	calls := 0
	providers := append(scope.Providers(), replayEvidenceProvider{calls: &calls, value: 100})
	ctx := request.WithQueryPolicy(context.Background(), request.QueryPolicy{IgnoreEmptyParameters: true})
	replay, err := selected.CaptureSources(ctx, providers)
	require.NoError(t, err)
	raw, err := json.Marshal(replay)
	require.NoError(t, err)
	require.JSONEq(t, `{"ID":7,"Names":["one","two"],"Token":"signed"}`, string(raw))
	require.Zero(t, calls)
	actual := &input{}
	require.NoError(t, injector.Bind(ctx, actual, WithPlan(plan), WithReplay(ReplayBinding{Replay: replay, Only: true})))
	require.Equal(t, 8, actual.Default)
	require.Zero(t, actual.Current)
	require.Equal(t, []string{"one", "two"}, actual.Names)
}
func TestReplayCaptureSourcesDirectAndDerivedCodec(t *testing.T) {
	for _, derived := range []bool{false, true} {
		name := "direct"
		if derived {
			name = "derived"
		}
		t.Run(name, func(t *testing.T) {
			type input struct {
				Raw     string
				Claim   int
				Current int
			}
			calls := 0
			specs := []BindingSpec{{Path: "Raw", Name: "Token", Location: state.Location{Kind: "header", In: "Authorization"}}, {Path: "Claim", Location: state.Location{Kind: "header", In: "Authorization"}, SourceType: reflect.TypeFor[string](), Transformer: replayCodec{&calls}}, {Path: "Current", Location: state.Location{Kind: "dependency"}}}
			selectedPaths := []string{"Claim"}
			if derived {
				specs[1].Location = state.Location{Kind: "param", In: "Token"}
				selectedPaths = []string{"Raw"}
			}
			if !derived {
				specs = specs[1:]
			}
			injector, err := NewInjector()
			require.NoError(t, err)
			plan, err := injector.CompilePlan(reflect.TypeFor[input](), specs...)
			require.NoError(t, err)
			selected, err := plan.Replay(selectedPaths...)
			require.NoError(t, err)
			projection, err := plan.Projection()
			require.NoError(t, err)
			selected, err = selected.Prepare(projection, "Claim")
			require.NoError(t, err)
			replay, err := selected.CaptureSources(context.Background(), request.NewValues(request.WithHeaders(map[string][]string{"Authorization": {"signed"}})).Providers())
			require.NoError(t, err)
			require.Zero(t, calls)
			raw, err := json.Marshal(replay)
			require.NoError(t, err)
			key := "Claim"
			if derived {
				key = "Token"
			}
			require.JSONEq(t, `{"`+key+`":"signed"}`, string(raw))
			// Full real JWT verification belongs to the Datly/Scy HTTP tests; this
			// proves capture never invokes the declared transformer itself.
		})
	}
}

type captureFault struct {
	cancel context.CancelFunc
	err    error
}

func (p captureFault) Kind() string                              { return "query" }
func (p captureFault) Priority() int                             { return 0 }
func (p captureFault) Locate(*structology.State) locator.Locator { return p }
func (p captureFault) Value(context.Context, reflect.Type, string) (any, bool, error) {
	if p.cancel != nil {
		p.cancel()
	}
	return 7, true, p.err
}
func TestReplayCaptureSourcesErrorsReturnNoPartialReplay(t *testing.T) {
	type input struct{ ID int }
	injector, err := NewInjector()
	require.NoError(t, err)
	plan, err := injector.CompilePlan(reflect.TypeFor[input](), BindingSpec{Path: "ID", Location: state.Location{Kind: "query", In: "id"}})
	require.NoError(t, err)
	selected, err := plan.Replay("ID")
	require.NoError(t, err)
	failure := errors.New("source failed")
	for _, mode := range []string{"cancel before", "cancel during", "provider error", "conversion", "nil provider"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			provider := locator.Provider(captureFault{})
			switch mode {
			case "cancel before":
				cancel()
			case "cancel during":
				provider = captureFault{cancel: cancel}
			case "provider error":
				provider = captureFault{err: failure}
			case "conversion":
				provider = values.New("query", map[string]any{"id": "bad"})
			case "nil provider":
				provider = nil
			}
			replay, err := selected.CaptureSources(ctx, []locator.Provider{provider})
			require.Error(t, err)
			require.Nil(t, replay)
			if strings.HasPrefix(mode, "cancel") {
				require.ErrorIs(t, err, context.Canceled)
			}
		})
	}
}

func TestReplayCaptureSourcesInKeepsScopeAndReadsFreshValues(t *testing.T) {
	type input struct{ Role int }
	root, err := NewInjector()
	require.NoError(t, err)
	plan, err := root.CompilePlan(reflect.TypeFor[input](), BindingSpec{Path: "Role", Location: state.Location{Kind: "header", In: "role"}})
	require.NoError(t, err)
	selected, err := plan.Replay("Role")
	require.NoError(t, err)
	provider := &countingProvider{kind: "header", cacheable: true}
	scope, err := root.ForScope(provider)
	require.NoError(t, err)
	for _, role := range []int{1, 2} {
		captured, err := selected.CaptureSourcesIn(t.Context(), scope)
		require.NoError(t, err)
		raw, err := captured.MarshalJSON()
		require.NoError(t, err)
		require.JSONEq(t, fmt.Sprintf(`{"Role":%d}`, role), string(raw))
	}
	other, err := root.ForScope(values.New("header", map[string]any{"role": 99}))
	require.NoError(t, err)
	captured, err := selected.CaptureSourcesIn(t.Context(), other)
	require.NoError(t, err)
	raw, err := captured.MarshalJSON()
	require.NoError(t, err)
	require.JSONEq(t, `{"Role":99}`, string(raw))
	captured, err = selected.CaptureSourcesIn(t.Context(), scope)
	require.NoError(t, err)
	raw, err = captured.MarshalJSON()
	require.NoError(t, err)
	require.JSONEq(t, `{"Role":3}`, string(raw))
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = selected.CaptureSourcesIn(canceled, scope)
	require.ErrorIs(t, err, context.Canceled)
}

func TestReplayNativeSourceEqualityPreservesValuesJSONNormalizes(t *testing.T) {
	type input struct{ Source any }
	root, err := NewInjector()
	require.NoError(t, err)
	plan, err := root.CompilePlan(reflect.TypeFor[input](), BindingSpec{Path: "Source", Location: state.Location{Kind: "header", In: "source"}})
	require.NoError(t, err)
	selected, err := plan.Replay("Source")
	require.NoError(t, err)
	capture := func(value any) *Replay {
		scope, err := root.ForScope(values.New("header", map[string]any{"source": value}))
		require.NoError(t, err)
		snapshot, err := selected.CaptureSourcesIn(t.Context(), scope)
		require.NoError(t, err)
		return snapshot
	}
	type privateScalar struct {
		Public  string
		private string
	}
	for _, pair := range [][2]any{{string([]byte{'x', 0xff}), string([]byte{'x', 0xfe})}, {privateScalar{"same", "first"}, privateScalar{"same", "second"}}} {
		first, second := capture(pair[0]), capture(pair[1])
		a, err := first.MarshalJSON()
		require.NoError(t, err)
		b, err := second.MarshalJSON()
		require.NoError(t, err)
		require.JSONEq(t, string(a), string(b), "the serialized representations collide")
		require.True(t, first.SourcesPresent())
		require.True(t, second.SourcesPresent())
		require.False(t, first.SourcesEqual(second))
		require.True(t, first.SourcesEqual(capture(pair[0])))
		decoded, err := selected.DecodeJSON(a)
		require.NoError(t, err)
		require.False(t, decoded.SourcesPresent())
		require.False(t, first.SourcesEqual(decoded))
	}
	type privateMutable struct{ private []int }
	opaque := capture(privateMutable{[]int{1}})
	require.False(t, opaque.SourcesPresent())
	require.False(t, opaque.SourcesEqual(opaque), "opaque data cannot establish source identity")
}
