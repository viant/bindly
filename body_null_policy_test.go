package bindly

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sync"
	"testing"

	"github.com/viant/bindly/locator"
	"github.com/viant/bindly/provider/body"
	"github.com/viant/bindly/provider/values"
	"github.com/viant/bindly/state"
)

type nullRecordMarker struct{ ID, Name, Notes, Child, Children bool }
type nullRecord struct {
	ID       int               `json:"id"`
	Name     string            `json:"name"`
	Notes    *string           `json:"notes"`
	Child    *nullRecord       `json:"child"`
	Children []*nullRecord     `json:"children"`
	Has      *nullRecordMarker `json:"-" setMarker:"true"`
}
type nullInput struct {
	View *nullRecord          `parameter:"kind=body,in=,required=true,bodyNullPolicy=empty-record"`
	Has  *struct{ View bool } `setMarker:"true"`
}

func nullPolicyPlan(t *testing.T, injector *Injector, policy string, required bool) *Plan {
	t.Helper()
	plan, err := injector.CompilePlan(reflect.TypeFor[nullInput](), BindingSpec{Path: "View", Name: "View", Location: state.Location{Kind: "body"}, Required: &required, BodyNullPolicy: policy})
	if err != nil {
		t.Fatal(err)
	}
	return plan
}
func assertEmptyNull(t *testing.T, input *nullInput) {
	t.Helper()
	if input.View == nil || input.View.Has == nil || *input.View.Has != (nullRecordMarker{}) || input.View.ID != 0 || input.Has == nil || !input.Has.View {
		t.Fatalf("empty supplied record missing original presence: %+v", input)
	}
}

func TestBodyNullPolicyMatrix(t *testing.T) {
	for _, required := range []bool{false, true} {
		for _, policy := range []string{"", "empty-record"} {
			for _, tc := range []struct {
				name, raw, media    string
				absent, bad, isNull bool
			}{
				{name: "absent", absent: true}, {name: "empty", absent: true}, {name: "whitespace", raw: " \n\t", bad: true},
				{name: "null", raw: "null", isNull: true}, {name: "whitespace null", raw: " \nnull\t", isNull: true},
				{name: "suffix json", raw: "null", media: "application/problem+json", isNull: true},
				{name: "object", raw: "{}"}, {name: "sparse", raw: `{"name":"x"}`},
				{name: "field null", raw: `{"notes":null}`}, {name: "nested null", raw: `{"child":null}`},
				{name: "array null", raw: `{"children":[null,{"name":"n"}]}`},
				{name: "scalar", raw: "1", bad: true}, {name: "malformed", raw: "{", bad: true}, {name: "trailing", raw: "null {}", bad: true},
				{name: "non json", raw: "null", media: "text/plain", bad: true},
			} {
				t.Run(tc.name+"/"+policy+"/"+map[bool]string{true: "required", false: "optional"}[required], func(t *testing.T) {
					media := tc.media
					if media == "" && tc.name != "null" {
						media = "application/json"
					}
					source, err := body.New([]byte(tc.raw), media, nil)
					if err != nil {
						t.Fatal(err)
					}
					injector, _ := NewInjector(WithProviders(source))
					plan := nullPolicyPlan(t, injector, policy, required)
					input := &nullInput{}
					err = injector.Bind(context.Background(), input, WithPlan(plan))
					reject := tc.bad || required && (tc.absent || tc.isNull && policy == "")
					if (err != nil) != reject {
						t.Fatalf("reject=%v err=%v", reject, err)
					}
					if reject {
						return
					}
					if tc.absent {
						if input.View != nil || input.Has != nil {
							t.Fatal("absence materialized")
						}
						return
					}
					if tc.isNull && policy == "" {
						if input.View != nil || input.Has == nil || !input.Has.View {
							t.Fatal("strict null changed")
						}
						return
					}
					if tc.isNull || tc.raw == "{}" {
						assertEmptyNull(t, input)
						return
					}
					if input.View.Has == nil || input.Has == nil || !input.Has.View {
						t.Fatal("source presence lost")
					}
					switch tc.name {
					case "sparse":
						if !input.View.Has.Name || input.View.Has.ID {
							t.Fatal("sparse flags")
						}
					case "field null":
						if !input.View.Has.Notes || input.View.Notes != nil {
							t.Fatal("field null")
						}
					case "nested null":
						if !input.View.Has.Child || input.View.Child != nil {
							t.Fatal("nested null")
						}
					case "array null":
						if !input.View.Has.Children || input.View.Children[0] != nil || input.View.Children[1].Has == nil || !input.View.Children[1].Has.Name {
							t.Fatal("array presence")
						}
					}
				})
			}
		}
	}
}

func TestBodyNullPolicyTagsAndValidation(t *testing.T) {
	injector, _ := NewInjector()
	plan, err := injector.CompilePlan(reflect.TypeFor[nullInput]())
	if err != nil || plan.bindings[0].BodyNullPolicy != "empty-record" {
		t.Fatalf("tag: %v", err)
	}
	field := reflect.StructField{Name: "View", Type: reflect.TypeFor[*nullRecord](), Tag: `bind:"bodyNullPolicy='empty-record',kind=body,in="`}
	parsed, found, err := BindingSpecFromField(field)
	if err != nil || !found || parsed.BodyNullPolicy != "empty-record" {
		t.Fatalf("bind tag: %+v %v", parsed, err)
	}
	valid := BindingSpec{Location: state.Location{Kind: "body"}, BodyNullPolicy: "empty-record"}
	for _, target := range []reflect.Type{nil, reflect.TypeFor[nullRecord](), reflect.TypeFor[**nullRecord](), reflect.TypeFor[*int](), reflect.TypeFor[map[string]int](), reflect.TypeFor[[]nullRecord](), reflect.TypeFor[[1]nullRecord]()} {
		if valid.ValidateBodyNullPolicy(target) == nil {
			t.Fatalf("accepted %v", target)
		}
	}
	for _, spec := range []BindingSpec{
		{BodyNullPolicy: "unknown", Location: state.Location{Kind: "body"}},
		{BodyNullPolicy: "empty-record", Location: state.Location{Kind: "query"}},
		{BodyNullPolicy: "empty-record", Location: state.Location{Kind: "body", In: "named"}},
		{BodyNullPolicy: "empty-record", Location: state.Location{Kind: "body"}, SourceType: reflect.TypeFor[*nullRecord]()},
		{BodyNullPolicy: "empty-record", Location: state.Location{Kind: "body"}, Transformer: recordTransformer{}},
	} {
		spec.Path = "View"
		if _, err := injector.CompilePlan(reflect.TypeFor[nullInput](), spec); err == nil {
			t.Fatalf("accepted %+v", spec)
		}
	}
	if _, has, err := plan.ExplicitPresence(&nullInput{}, "View"); err != nil || !has {
		t.Fatal("compiled nil marker unavailable")
	}
	type markerless struct{ View *nullRecord }
	bare, err := injector.CompilePlan(reflect.TypeFor[markerless](), BindingSpec{Path: "View", Location: state.Location{Kind: "body"}})
	if err != nil {
		t.Fatal(err)
	}
	if present, has, err := bare.ExplicitPresence(&markerless{}, "View"); err != nil || has || present {
		t.Fatal("markerless proves suppliedness")
	}
}

type recordTransformer struct{}

func (recordTransformer) Transform(context.Context, locator.Resolver, any) (any, error) {
	return nil, nil
}

func TestBodyNullPolicyCacheIsolationAndConcurrency(t *testing.T) {
	source, _ := body.New([]byte("null"), "application/json", nil)
	injector, _ := NewInjector(WithProviders(source))
	opt := nullPolicyPlan(t, injector, "empty-record", true)
	strict := nullPolicyPlan(t, injector, "", true)
	cache := NewValueCache()
	for i := 0; i < 3; i++ {
		input := &nullInput{}
		if err := injector.Bind(context.Background(), input, WithPlan(opt), func(o *bindOptions) { o.cache = cache }); err != nil {
			t.Fatal(err)
		}
		assertEmptyNull(t, input)
		input.View.Name = "mutated"
		if err := injector.Bind(context.Background(), &nullInput{}, WithPlan(strict), func(o *bindOptions) { o.cache = cache }); err == nil {
			t.Fatal("opt contaminated strict")
		}
	}
	var wg sync.WaitGroup
	records := make(chan *nullRecord, 32)
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			input := &nullInput{}
			if err := injector.Bind(context.Background(), input, WithPlan(opt), func(o *bindOptions) { o.cache = cache }); err != nil {
				t.Error(err)
				return
			}
			assertEmptyNull(t, input)
			records <- input.View
		}()
	}
	wg.Wait()
	close(records)
	seen := map[*nullRecord]bool{}
	for item := range records {
		if seen[item] {
			t.Fatal("shared allocation")
		}
		seen[item] = true
	}
	type mixed struct {
		Strict *nullRecord
		Opt    *nullRecord
	}
	no := false
	mixedPlan, err := injector.CompilePlan(reflect.TypeFor[mixed](), BindingSpec{Path: "Strict", Location: state.Location{Kind: "body"}, Required: &no}, BindingSpec{Path: "Opt", Location: state.Location{Kind: "body"}, BodyNullPolicy: "empty-record"})
	if err != nil {
		t.Fatal(err)
	}
	actual := &mixed{}
	if err = injector.Bind(context.Background(), actual, WithPlan(mixedPlan)); err != nil || actual.Strict != nil || actual.Opt == nil {
		t.Fatalf("mixed policy: %+v %v", actual, err)
	}
	unsupported, _ := NewInjector(WithProviders(values.New("body", map[string]any{"": nil})))
	if err := unsupported.Bind(context.Background(), &nullInput{}, WithPlan(opt)); err == nil {
		t.Fatal("arbitrary nil normalized")
	}
}

func TestBodyNullPolicyDeferredErrorsAndCancellation(t *testing.T) {
	cause := errors.New("body read failure")
	loads := 0
	source := body.NewDeferred(func(context.Context) (*body.Source, error) { loads++; return nil, cause })
	injector, _ := NewInjector(WithProviders(source))
	plan := nullPolicyPlan(t, injector, "empty-record", true)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := injector.Bind(ctx, &nullInput{}, WithPlan(plan)); !errors.Is(err, context.Canceled) || loads != 0 {
		t.Fatal("cancellation consumed body")
	}
	for i := 0; i < 2; i++ {
		if err := injector.Bind(context.Background(), &nullInput{}, WithPlan(plan)); !errors.Is(err, cause) {
			t.Fatal(err)
		}
	}
	if loads != 1 {
		t.Fatal("deferred error retried")
	}
}

func TestBodyNullPolicyReplayPreservesNull(t *testing.T) {
	ctx := context.Background()
	source, _ := body.New([]byte(" null "), "application/json", nil)
	injector, _ := NewInjector(WithProviders(source))
	plan := nullPolicyPlan(t, injector, "empty-record", true)
	selected, err := plan.Replay("View")
	if err != nil {
		t.Fatal(err)
	}
	replay, err := selected.CaptureSources(ctx, []locator.Provider{source})
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(replay)
	if err != nil || string(data) != `{"View":null}` {
		t.Fatalf("raw capture %s %v", data, err)
	}
	replay, err = selected.DecodeJSON(data)
	if err != nil {
		t.Fatal(err)
	}
	first := &nullInput{}
	if err = injector.Bind(ctx, first, WithPlan(plan), WithReplay(ReplayBinding{Replay: replay, Only: true})); err != nil {
		t.Fatal(err)
	}
	assertEmptyNull(t, first)
	data, err = json.Marshal(replay)
	if err != nil || string(data) != `{"View":null}` {
		t.Fatalf("capturePrepared changed raw %s %v", data, err)
	}
	providers, err := replay.Providers()
	if err != nil {
		t.Fatal(err)
	}
	dependency, _ := NewInjector(WithProviders(providers...))
	second := &nullInput{}
	if err = dependency.Bind(ctx, second, WithPlan(plan)); err != nil {
		t.Fatal(err)
	}
	assertEmptyNull(t, second)
	if second.View == first.View || second.View.Has == first.View.Has {
		t.Fatal("dependency shared prepared object")
	}
	strict := nullPolicyPlan(t, dependency, "", true)
	if err = dependency.Bind(ctx, &nullInput{}, WithPlan(strict)); err == nil {
		t.Fatal("strict dependency consumed normalized parent")
	}
	composed, err := locator.ComposeProviders("body", locator.ProviderLayer{Name: "replay", Provider: providers[0]}, locator.ProviderLayer{Name: "body", Provider: source})
	if err != nil {
		t.Fatal(err)
	}
	composedInjector, _ := NewInjector(WithProviders(composed))
	if err = composedInjector.Bind(ctx, &nullInput{}, WithPlan(plan)); err != nil {
		t.Fatal(err)
	}
	projected, err := selected.CaptureSources(ctx, providers)
	if err != nil {
		t.Fatal(err)
	}
	data, _ = json.Marshal(projected)
	if string(data) != `{"View":null}` {
		t.Fatalf("projection fabricated keys %s", data)
	}
	first.View.Name = "working mutation"
	first.View.Has.Name = true
	retry := &nullInput{}
	if err = injector.Bind(ctx, retry, WithPlan(plan), WithReplay(ReplayBinding{Replay: replay})); err != nil {
		t.Fatal(err)
	}
	assertEmptyNull(t, retry)
	if retry.View == first.View || retry.View.Has == first.View.Has {
		t.Fatal("retry shared object")
	}
	// Explicit typed null is representable; an already normalized object needs
	// raw source evidence because serializing its zero fields invents presence.
	explicit := &nullInput{Has: &struct{ View bool }{View: true}}
	captured, err := selected.Capture(explicit)
	if err != nil {
		t.Fatal(err)
	}
	data, _ = json.Marshal(captured)
	if string(data) != `{"View":null}` {
		t.Fatalf("typed null %s", data)
	}
	if _, err = selected.Capture(retry); err == nil {
		t.Fatal("normalized object fabricated source presence")
	}
	absent, err := selected.DecodeJSON([]byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if err = injector.Bind(ctx, &nullInput{}, WithPlan(plan), WithReplay(ReplayBinding{Replay: absent})); err == nil {
		t.Fatal("replay absence normalized")
	}
}

func TestBodyNullPolicyNativeDecoderEvidence(t *testing.T) {
	ctx := context.Background()
	target := reflect.TypeFor[*nullRecord]()
	invalid, _ := body.New([]byte("\u00a0null"), "application/json", nil)
	if _, _, err := invalid.ValueWithBodyNullPolicy(ctx, target, "", "empty-record"); err == nil {
		t.Fatal("non-JSON whitespace normalized")
	}
	aliased, _ := body.New([]byte(`{"view":null}`), "application/json", map[string]string{"": "view"})
	if _, _, err := aliased.ValueWithBodyNullPolicy(ctx, target, "", "empty-record"); err == nil {
		t.Fatal("named alias normalized")
	}
	for _, name := range []string{"view", "missing"} {
		source, _ := body.New([]byte(`{"view":null}`), "application/json", nil)
		if _, _, err := source.ValueWithBodyNullPolicy(ctx, target, name, "empty-record"); err == nil {
			t.Fatal("named body policy accepted")
		}
	}
	if _, err := body.NewEmptyRecord(reflect.TypeFor[**nullRecord]()); err == nil {
		t.Fatal("helper accepted pointer chain")
	}
	first, err := body.NewEmptyRecord(target)
	if err != nil {
		t.Fatal(err)
	}
	second, err := body.NewEmptyRecord(target)
	if err != nil {
		t.Fatal(err)
	}
	if first == second || first.(*nullRecord).Has == nil || first.(*nullRecord).Has == second.(*nullRecord).Has {
		t.Fatal("native helper does not initialize detached original markers")
	}
	loads := 0
	deferred := body.NewDeferred(func(context.Context) (*body.Source, error) {
		loads++
		return body.New([]byte("null"), "application/json", nil)
	})
	value, present, err := deferred.ValueWithBodyNullPolicy(ctx, target, "", "empty-record")
	if err != nil || !present || value == nil {
		t.Fatal("deferred opted null")
	}
	strict, present, err := deferred.Value(ctx, target, "")
	if err != nil || !present || strict != nil || loads != 1 {
		t.Fatal("deferred policy leaked")
	}
	raw, present, err := deferred.CaptureSource(ctx, target, "")
	if err != nil || !present || raw != nil {
		t.Fatal("deferred raw null changed")
	}
	// Default behavior remains identical for unmarked non-body required bindings.
	required := true
	injector, _ := NewInjector(WithProviders(values.New("query", map[string]any{"id": nil})))
	type queryInput struct{ ID *int }
	plan, err := injector.CompilePlan(reflect.TypeFor[queryInput](), BindingSpec{Path: "ID", Location: state.Location{Kind: "query", In: "id"}, Required: &required})
	if err != nil {
		t.Fatal(err)
	}
	if err = injector.Bind(ctx, &queryInput{}, WithPlan(plan)); err == nil {
		t.Fatal("non-body required nil relaxed")
	}
}
