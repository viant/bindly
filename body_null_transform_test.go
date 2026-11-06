package bindly

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/viant/bindly/locator"
	"github.com/viant/bindly/provider/body"
	"github.com/viant/bindly/state"
)

type rawRecordDecoder struct{ calls int }

func (d *rawRecordDecoder) Transform(ctx context.Context, _ locator.Resolver, value any) (any, error) {
	d.calls++
	raw, ok := value.(json.RawMessage)
	if !ok {
		return nil, fmt.Errorf("expected raw JSON, got %T", value)
	}
	source, err := body.New(raw, "application/json", nil)
	if err != nil {
		return nil, err
	}
	result, _, err := source.Value(ctx, reflect.TypeFor[*nullRecord](), "")
	return result, err
}
func TestRawDecoderBodyNullPolicy(t *testing.T) {
	for _, replayMode := range []bool{false, true} {
		for _, required := range []bool{false, true} {
			for _, tc := range []struct {
				name, raw string
				invalid   bool
			}{
				{"absent", "", false}, {"null", "null", false}, {"object", "{}", false},
				{"zero", `{"id":0,"name":""}`, false}, {"malformed", "{", true},
			} {
				t.Run(fmt.Sprintf("replay=%v/required=%v/%s", replayMode, required, tc.name), func(t *testing.T) {
					decoder := &rawRecordDecoder{}
					injector, _ := NewInjector()
					plan, err := injector.CompilePlan(reflect.TypeFor[nullInput](), BindingSpec{Path: "View", Name: "View", Location: state.Location{Kind: "body"}, Required: &required, BodyNullPolicy: "empty-record", SourceType: reflect.TypeFor[json.RawMessage](), Transformer: decoder})
					if err != nil {
						t.Fatal(err)
					}
					source, err := body.New([]byte(tc.raw), "application/json", nil)
					if err != nil {
						t.Fatal(err)
					}
					scope, err := injector.ForScope(source)
					if err != nil {
						t.Fatal(err)
					}
					options := []BindOption{WithPlan(plan)}
					if replayMode {
						selected, e := plan.Replay("View")
						if e != nil {
							t.Fatal(e)
						}
						captured, e := selected.CaptureSources(t.Context(), []locator.Provider{source})
						if e != nil {
							if tc.invalid {
								return
							}
							t.Fatal(e)
						}
						options = append(options, WithReplay(ReplayBinding{Replay: captured, Only: true}))
					}
					var actual nullInput
					err = scope.Bind(t.Context(), &actual, options...)
					if tc.invalid || required && tc.name == "absent" {
						if err == nil {
							t.Fatal("invalid body accepted")
						}
						return
					}
					if err != nil {
						t.Fatal(err)
					}
					if tc.name == "absent" {
						if actual.View != nil || decoder.calls != 0 {
							t.Fatal("absence became a value")
						}
						return
					}
					if tc.name == "null" {
						assertEmptyNull(t, &actual)
						if decoder.calls != 0 {
							t.Fatal("null called decoder")
						}
						return
					}
					if decoder.calls != 1 || actual.View == nil || actual.View.Has == nil || actual.Has == nil || !actual.Has.View {
						t.Fatalf("decoder/presence lost: calls=%d actual=%+v", decoder.calls, actual)
					}
					if tc.name == "zero" && (!actual.View.Has.ID || !actual.View.Has.Name) {
						t.Fatal("supplied zero lost")
					}
				})
			}
		}
	}
}

func TestRawNullPolicyDoesNotContaminateStrictCache(t *testing.T) {
	source, _ := body.New([]byte("null"), "application/json", nil)
	injector, _ := NewInjector(WithProviders(source))
	required := true
	decoder := &rawRecordDecoder{}
	spec := BindingSpec{Path: "View", Location: state.Location{Kind: "body"}, Required: &required, SourceType: reflect.TypeFor[json.RawMessage](), Transformer: decoder, BodyNullPolicy: "empty-record"}
	opted, err := injector.CompilePlan(reflect.TypeFor[nullInput](), spec)
	if err != nil {
		t.Fatal(err)
	}
	spec.BodyNullPolicy = ""
	strict, err := injector.CompilePlan(reflect.TypeFor[nullInput](), spec)
	if err != nil {
		t.Fatal(err)
	}
	cache := NewValueCache()
	cacheOption := func(o *bindOptions) { o.cache = cache }
	var previous *nullRecord
	for i := 0; i < 3; i++ {
		var actual nullInput
		if err := injector.Bind(t.Context(), &actual, WithPlan(opted), cacheOption); err != nil {
			t.Fatal(err)
		}
		assertEmptyNull(t, &actual)
		if actual.View == previous {
			t.Fatal("normalized instance retained in source cache")
		}
		previous = actual.View
		actual.View.Name = "changed"
		if err := injector.Bind(t.Context(), &nullInput{}, WithPlan(strict), cacheOption); err == nil {
			t.Fatal("opted binding contaminated strict binding")
		}
	}
	if decoder.calls != 0 {
		t.Fatal("null reached custom decoder")
	}
}
