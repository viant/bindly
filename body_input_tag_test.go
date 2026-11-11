package bindly

import (
	"context"
	"encoding/json"
	"github.com/viant/bindly/locator"
	"github.com/viant/bindly/provider/body"
	"reflect"
	"testing"
)

type rootPair struct {
	ID     *int                       `json:"id"`
	Secret string                     `internal:"true"`
	Has    *struct{ ID, Secret bool } `json:"-" setMarker:"true"`
}
type rootPairsInput struct {
	Body []rootPair           `parameter:"kind=body,in=" jsonx:"objectToSlice"`
	Has  *struct{ Body bool } `json:"-" setMarker:"true"`
}
type strictPairsInput struct {
	Body []rootPair `parameter:"kind=body,in="`
}

func TestBodyInputTagObjectToSlice(t *testing.T) {
	for _, deferred := range []bool{false, true} {
		for _, tc := range []struct {
			raw                    string
			count                  int
			present, nilSlice, bad bool
		}{
			{``, 0, false, true, false}, {`null`, 0, true, true, false}, {`[]`, 0, true, false, false},
			{`{}`, 1, true, false, false}, {`{"id":0,"Secret":"forged","Has":{"Secret":true}}`, 1, true, false, false},
			{`[{"id":1},{"id":2}]`, 2, true, false, false}, {`[null]`, 1, true, false, false},
			{`1`, 0, false, false, true}, {`[1]`, 0, false, false, true}, {`{} {}`, 0, false, false, true},
		} {
			source, _ := body.New([]byte(tc.raw), "application/json", nil)
			if deferred {
				original := source
				source = body.NewDeferred(func(context.Context) (*body.Source, error) { return original, nil })
			}
			injector, err := NewInjector(WithProviders(source))
			if err != nil {
				t.Fatal(err)
			}
			var input rootPairsInput
			err = injector.Bind(context.Background(), &input)
			if (err != nil) != tc.bad {
				t.Fatalf("deferred %v raw %s err %v", deferred, tc.raw, err)
			}
			if tc.bad {
				continue
			}
			present := input.Has != nil && input.Has.Body
			if len(input.Body) != tc.count || (input.Body == nil) != tc.nilSlice || present != tc.present {
				t.Fatalf("raw %s body %+v presence %v", tc.raw, input.Body, present)
			}
			for _, pair := range input.Body {
				if pair.Secret != "" || pair.Has != nil && pair.Has.Secret {
					t.Fatal("hidden field accepted")
				}
			}
		}
	}
}

func TestBodyInputTagStrictIsolation(t *testing.T) {
	source, _ := body.New([]byte(`{"id":1}`), "application/json", nil)
	injector, _ := NewInjector(WithProviders(source))
	var opted rootPairsInput
	if err := injector.Bind(context.Background(), &opted); err != nil {
		t.Fatal(err)
	}
	var strict strictPairsInput
	if err := injector.Bind(context.Background(), &strict); err == nil {
		t.Fatal("root policy leaked to strict slice")
	}
	type mixed struct {
		Opt    []rootPair `parameter:"kind=body,in=" jsonx:"objectToSlice"`
		Strict []rootPair `parameter:"kind=body,in="`
	}
	if err := injector.Bind(context.Background(), &mixed{}); err == nil {
		t.Fatal("invocation cache leaked root policy")
	}
}

func TestBodyInputTagRejectInvalidContract(t *testing.T) {
	injector, _ := NewInjector()
	for _, typ := range []reflect.Type{
		reflect.TypeOf(struct {
			Body *rootPair `parameter:"kind=body" jsonx:"objectToSlice"`
		}{}),
		reflect.TypeOf(struct {
			Body []rootPair `parameter:"kind=body" jsonx:"objectToSlice,unknown"`
		}{}),
	} {
		if _, err := injector.CompilePlan(typ); err == nil {
			t.Fatalf("accepted invalid contract %v", typ)
		}
	}
}

func TestBodyInputTagNestedPolicyAndPresence(t *testing.T) {
	type envelope struct {
		Pairs []rootPair `json:"pairs" jsonx:"objectToSlice"`
	}
	type input struct {
		Body *envelope `parameter:"kind=body"`
	}
	source, _ := body.New([]byte(`{"pairs":{"id":null,"Secret":"forged","Has":{"Secret":true}}}`), "application/json", nil)
	injector, _ := NewInjector(WithProviders(source))
	var actual input
	if err := injector.Bind(context.Background(), &actual); err != nil {
		t.Fatal(err)
	}
	if actual.Body == nil || len(actual.Body.Pairs) != 1 {
		t.Fatalf("missing nested record: %+v", actual.Body)
	}
	pair := actual.Body.Pairs[0]
	if pair.Has == nil || !pair.Has.ID || pair.Has.Secret || pair.ID != nil || pair.Secret != "" {
		t.Fatalf("incorrect nested presence %+v", pair)
	}
}

func TestBodyInputTagRequiredAndNullOmission(t *testing.T) {
	type required struct {
		Body []rootPair `parameter:"kind=body,required=true" jsonx:"objectToSlice"`
	}
	type omitted struct {
		Body []rootPair           `parameter:"kind=body" jsonx:"objectToSlice,null=omit"`
		Has  *struct{ Body bool } `json:"-" setMarker:"true"`
	}
	for _, raw := range []string{``, `null`} {
		source, _ := body.New([]byte(raw), "application/json", nil)
		injector, _ := NewInjector(WithProviders(source))
		if err := injector.Bind(context.Background(), &required{}); err == nil {
			t.Fatalf("required accepted %q", raw)
		}
		var input omitted
		if err := injector.Bind(context.Background(), &input); err != nil {
			t.Fatal(err)
		}
		if input.Body != nil || input.Has != nil && input.Has.Body {
			t.Fatal("omission became present")
		}
	}
}

func TestBodyInputTagReplayCompositionAndCache(t *testing.T) {
	ctx := context.Background()
	source, _ := body.New([]byte(`{"id":1}`), "application/json", nil)
	injector, _ := NewInjector(WithProviders(source))
	plan, err := injector.CompilePlan(reflect.TypeFor[rootPairsInput]())
	if err != nil {
		t.Fatal(err)
	}
	selected, err := plan.Replay("Body")
	if err != nil {
		t.Fatal(err)
	}
	replay, err := selected.CaptureSources(ctx, []locator.Provider{source})
	if err != nil {
		t.Fatal(err)
	}
	var actual rootPairsInput
	if err = injector.Bind(ctx, &actual, WithPlan(plan), WithReplay(ReplayBinding{Replay: replay, Only: true})); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(replay)
	if err != nil || string(raw) != `{"Body":{"id":1}}` {
		t.Fatalf("raw replay changed %s %v", raw, err)
	}
	providers, err := replay.Providers()
	if err != nil {
		t.Fatal(err)
	}
	composed, err := locator.ComposeProviders("body", locator.ProviderLayer{Name: "replay", Provider: providers[0]}, locator.ProviderLayer{Name: "body", Provider: source})
	if err != nil {
		t.Fatal(err)
	}
	consumer, _ := NewInjector(WithProviders(composed))
	cache := NewValueCache()
	for i := 0; i < 2; i++ {
		var opted rootPairsInput
		if err := consumer.Bind(ctx, &opted, func(o *bindOptions) { o.cache = cache }); err != nil {
			t.Fatal(err)
		}
		if len(opted.Body) != 1 || opted.Body[0].ID == nil || *opted.Body[0].ID != 1 {
			t.Fatalf("bad replay %+v", opted.Body)
		}
		if err := consumer.Bind(ctx, &strictPairsInput{}, func(o *bindOptions) { o.cache = cache }); err == nil {
			t.Fatal("strict replay consumed prepared collection")
		}
	}
	// Also isolate persistent entries from the directly cacheable body source.
	cache = NewValueCache()
	if err := injector.Bind(ctx, &rootPairsInput{}, func(o *bindOptions) { o.cache = cache }); err != nil {
		t.Fatal(err)
	}
	if err := injector.Bind(ctx, &strictPairsInput{}, func(o *bindOptions) { o.cache = cache }); err == nil {
		t.Fatal("persistent policy cache contaminated strict body")
	}
}
