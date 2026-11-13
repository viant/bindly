package bindly

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/viant/bindly/input"
	"github.com/viant/bindly/locator"
	"github.com/viant/bindly/provider/body"
	"github.com/viant/bindly/provider/values"
	"github.com/viant/bindly/state"
	"github.com/viant/structology"
	structjson "github.com/viant/structology/encoding/json"
	"github.com/viant/x/shape"
)

type representedRecord struct {
	Q      *string                   `json:"q" jsonx:"numberToString"`
	Secret string                    `json:"secret"`
	Has    *struct{ Q, Secret bool } `json:"-" setMarker:"true"`
}
type representedInput struct{ Body *representedRecord }

type representedDecoderOwner struct{ calls atomic.Int32 }

func (d *representedDecoderOwner) Decode(raw []byte, target any) error {
	d.calls.Add(1)
	return structjson.UnmarshalStandard(raw, target)
}
func (d *representedDecoderOwner) UnmarshalWithRepresentation(raw []byte, target any, metadata *shape.Representation) error {
	d.calls.Add(1)
	return structjson.UnmarshalStandard(raw, target, structjson.WithRepresentation(metadata))
}

func bodyMetadata(t *testing.T) *shape.Representation {
	t.Helper()
	walker, err := shape.CompileRewrite(reflect.TypeFor[*representedRecord]())
	require.NoError(t, err)
	metadata, err := walker.RewriteRepresentation(func(_ string, _ reflect.Type, field *shape.FieldMetadata) (*shape.FieldMetadata, error) {
		if field.Name == "Secret" {
			return &shape.FieldMetadata{Name: field.Name, Tag: `json:"-" internal:"true"`}, nil
		}
		if field.Name == "Q" {
			return &shape.FieldMetadata{Name: field.Name, Tag: `json:"q" jsonx:"numberToString" required:"true"`}, nil
		}
		return nil, nil
	})
	require.NoError(t, err)
	return metadata
}

type countedBody struct {
	*body.Source
	reads atomic.Int32
}

func (s *countedBody) Locate(*structology.State) locator.Locator { return s }
func (s *countedBody) Value(ctx context.Context, target reflect.Type, name string) (any, bool, error) {
	s.reads.Add(1)
	return s.Source.Value(ctx, target, name)
}
func (s *countedBody) ValueWithBodyRepresentation(ctx context.Context, target reflect.Type, name string, metadata *shape.Representation, policy string, tag reflect.StructTag) (any, bool, error) {
	s.reads.Add(1)
	return s.Source.ValueWithBodyRepresentation(ctx, target, name, metadata, policy, tag)
}

func TestBodyRepresentationCacheScopeAndConcurrentIsolation(t *testing.T) {
	metadata := bodyMetadata(t)
	source, err := body.New([]byte(`{"q":50,"secret":"canonical"}`), "application/json", nil)
	require.NoError(t, err)
	counted := &countedBody{Source: source}
	composed, err := locator.ComposeProviders("body", locator.ProviderLayer{Name: "request", Provider: counted})
	require.NoError(t, err)
	injector, err := NewInjector(WithProviders(composed))
	require.NoError(t, err)
	canonical, err := injector.CompilePlan(reflect.TypeFor[representedInput](), BindingSpec{Path: "Body", Location: state.Location{Kind: "body"}})
	require.NoError(t, err)
	represented, err := injector.CompilePlan(reflect.TypeFor[representedInput](), BindingSpec{Path: "Body", Location: state.Location{Kind: "body"}, BodyRepresentation: metadata})
	require.NoError(t, err)
	var workers sync.WaitGroup
	for index := 0; index < 16; index++ {
		workers.Add(1)
		go func(index int) {
			defer workers.Done()
			plan := canonical
			if index%2 == 1 {
				plan = represented
			}
			actual := &representedInput{}
			if err := injector.Bind(t.Context(), actual, WithPlan(plan)); err != nil {
				t.Error(err)
				return
			}
			if actual.Body == nil || actual.Body.Q == nil || *actual.Body.Q != "50" || actual.Body.Has == nil || !actual.Body.Has.Q {
				t.Errorf("lost native value/presence: %+v", actual.Body)
				return
			}
			wantSecret := "canonical"
			if index%2 == 1 {
				wantSecret = ""
			}
			if actual.Body.Secret != wantSecret || actual.Body.Has.Secret != (index%2 == 0) {
				t.Errorf("representation cache crossed scope: %+v", actual.Body)
			}
		}(index)
	}
	workers.Wait()
	require.EqualValues(t, 2, counted.reads.Load(), "one native lookup per canonical/represented cache identity")
	for _, plan := range []*Plan{canonical, represented} {
		actual := &representedInput{}
		require.NoError(t, injector.Bind(t.Context(), actual, WithPlan(plan)))
	}
	require.EqualValues(t, 2, counted.reads.Load(), "warm plans reuse native source results")
	equivalent, err := injector.CompilePlan(reflect.TypeFor[representedInput](), BindingSpec{Path: "Body", Location: state.Location{Kind: "body"}, BodyRepresentation: bodyMetadata(t)})
	require.NoError(t, err)
	require.NoError(t, injector.Bind(t.Context(), &representedInput{}, WithPlan(equivalent)))
	require.EqualValues(t, 2, counted.reads.Load(), "equivalent metadata retains deterministic cache identity")
	walker, err := shape.CompileRewrite(reflect.TypeFor[*representedRecord]())
	require.NoError(t, err)
	alternative, err := walker.RewriteRepresentation(func(_ string, _ reflect.Type, field *shape.FieldMetadata) (*shape.FieldMetadata, error) {
		if field.Name == "Q" {
			return &shape.FieldMetadata{Name: field.Name, Tag: `json:"-" internal:"true"`}, nil
		}
		return nil, nil
	})
	require.NoError(t, err)
	revised, err := injector.CompilePlan(reflect.TypeFor[representedInput](), BindingSpec{Path: "Body", Location: state.Location{Kind: "body"}, BodyRepresentation: alternative})
	require.NoError(t, err)
	revisedInput := &representedInput{}
	require.NoError(t, injector.Bind(t.Context(), revisedInput, WithPlan(revised)))
	require.Nil(t, revisedInput.Body.Q)
	require.False(t, revisedInput.Body.Has.Q)
	require.Equal(t, "canonical", revisedInput.Body.Secret)
	require.EqualValues(t, 3, counted.reads.Load(), "changed metadata invalidates only its affected source entry")
	scoped, err := injector.ForScope(composed)
	require.NoError(t, err)
	require.NoError(t, scoped.Bind(t.Context(), &representedInput{}, WithPlan(represented)))
	require.EqualValues(t, 4, counted.reads.Load(), "new scope invalidates prior source values")
}

func TestBodyRepresentationNamedDeferredPolicyAndReplay(t *testing.T) {
	metadata := bodyMetadata(t)
	for _, strict := range []bool{false, true} {
		for _, unknown := range []bool{false, true} {
			raw := `{"payload":{"q":50,"secret":"ignored"}}`
			if unknown {
				raw = `{"payload":{"q":50,"secret":"ignored","unexpected":true}}`
			}
			var loads atomic.Int32
			source := body.NewDeferred(func(context.Context) (*body.Source, error) {
				loads.Add(1)
				policy := structjson.IgnoreUnknown
				if strict {
					policy = structjson.ErrorOnUnknown
				}
				return body.New([]byte(raw), "application/json", map[string]string{"alias": "payload"}, body.WithUnknownFieldPolicy(policy))
			})
			injector, err := NewInjector(WithProviders(source))
			require.NoError(t, err)
			plan, err := injector.CompilePlan(reflect.TypeFor[representedInput](), BindingSpec{Path: "Body", Location: state.Location{Kind: "body", In: "alias"}, BodyRepresentation: metadata})
			require.NoError(t, err)
			actual := &representedInput{}
			err = injector.Bind(t.Context(), actual, WithPlan(plan))
			if strict && unknown {
				var invalid *input.Error
				require.ErrorAs(t, err, &invalid)
				require.Contains(t, invalid.Cause.Error(), `unknown field "unexpected"`)
			} else {
				require.NoError(t, err)
				require.Equal(t, "50", *actual.Body.Q)
				require.Empty(t, actual.Body.Secret)
				require.False(t, actual.Body.Has.Secret)
			}
			require.EqualValues(t, 1, loads.Load())
		}
	}
	injector, err := NewInjector()
	require.NoError(t, err)
	plan, err := injector.CompilePlan(reflect.TypeFor[representedInput](), BindingSpec{Path: "Body", Location: state.Location{Kind: "body", In: "payload"}, BodyRepresentation: metadata})
	require.NoError(t, err)
	replayPlan, err := plan.Replay("Body")
	require.NoError(t, err)
	source, err := body.New([]byte(`{"payload":{"q":50,"secret":"ignored"}}`), "application/json", nil)
	require.NoError(t, err)
	replay, err := replayPlan.CaptureSources(t.Context(), []locator.Provider{source})
	require.NoError(t, err)
	encoded, err := json.Marshal(replay)
	require.NoError(t, err)
	require.JSONEq(t, `{"Body":{"q":50,"secret":"ignored"}}`, string(encoded), "capture retains raw source")
	actual := &representedInput{}
	require.NoError(t, injector.Bind(t.Context(), actual, WithPlan(plan), WithReplay(ReplayBinding{Replay: replay, Only: true})))
	require.Equal(t, "50", *actual.Body.Q)
	require.Empty(t, actual.Body.Secret)
	require.False(t, actual.Body.Has.Secret)
	_, err = replayPlan.Capture(actual)
	require.ErrorContains(t, err, "requires raw source JSON")
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	failed := body.NewDeferred(func(context.Context) (*body.Source, error) { t.Fatal("cancelled source loaded"); return nil, nil })
	_, _, err = failed.ValueWithBodyRepresentation(cancelled, reflect.TypeFor[*representedRecord](), "", metadata, "", "")
	require.True(t, errors.Is(err, context.Canceled))
	_, _, err = source.ValueWithBodyRepresentation(t.Context(), reflect.TypeFor[*representedRecord](), "payload", metadata, "", "")
	require.NoError(t, err)
	missing, err := body.New([]byte(`{"secret":"ignored"}`), "application/json", nil)
	require.NoError(t, err)
	_, _, err = missing.ValueWithBodyRepresentation(t.Context(), reflect.TypeFor[*representedRecord](), "", metadata, "", "")
	require.Error(t, err, "effective required metadata remains enforced")
}

func TestBodyRepresentationRetainsConfiguredDecoderOwner(t *testing.T) {
	type record struct {
		Q      string                    `json:"q"`
		Secret string                    `json:"secret"`
		Has    *struct{ Q, Secret bool } `json:"-" setMarker:"true"`
	}
	type input struct{ Body *record }
	walker, err := shape.CompileRewrite(reflect.TypeFor[*record]())
	require.NoError(t, err)
	metadata, err := walker.RewriteRepresentation(func(_ string, _ reflect.Type, field *shape.FieldMetadata) (*shape.FieldMetadata, error) {
		if field.Name == "Secret" {
			return &shape.FieldMetadata{Name: field.Name, Tag: `json:"-" internal:"true"`}, nil
		}
		return nil, nil
	})
	require.NoError(t, err)
	owner := &representedDecoderOwner{}
	decoders := body.NewDecoderRegistry()
	decoders.Register("application/json", owner)
	source, err := body.New([]byte(`{"payload":{"q":"50","secret":"ignored","unknown":"owner accepts"}}`), "application/json", nil, body.WithDecoders(decoders))
	require.NoError(t, err)
	injector, err := NewInjector(WithProviders(source))
	require.NoError(t, err)
	plan, err := injector.CompilePlan(reflect.TypeFor[input](), BindingSpec{Path: "Body", Location: state.Location{Kind: "body", In: "payload"}, BodyRepresentation: metadata})
	require.NoError(t, err)
	actual := &input{}
	require.NoError(t, injector.Bind(t.Context(), actual, WithPlan(plan)))
	require.EqualValues(t, 1, owner.calls.Load(), "original configured decoder executes once")
	require.Equal(t, "50", actual.Body.Q)
	require.Empty(t, actual.Body.Secret)
	require.True(t, actual.Body.Has.Q)
	require.False(t, actual.Body.Has.Secret)
}

func TestBodyRepresentationRejectsIncompatibleTargetsAndLocators(t *testing.T) {
	metadata := bodyMetadata(t)
	injector, err := NewInjector()
	require.NoError(t, err)
	for _, spec := range []BindingSpec{
		{Path: "Body", Location: state.Location{Kind: "query"}, BodyRepresentation: metadata},
		{Path: "Body", Location: state.Location{Kind: "body"}, SourceType: reflect.TypeFor[string](), BodyRepresentation: metadata},
	} {
		_, err := injector.CompilePlan(reflect.TypeFor[representedInput](), spec)
		require.ErrorContains(t, err, "body representation must match its native body source type")
	}
	plan, err := injector.CompilePlan(reflect.TypeFor[representedInput](), BindingSpec{Path: "Body", Location: state.Location{Kind: "body"}, BodyRepresentation: metadata})
	require.NoError(t, err)
	unsupported := values.New("body", map[string]any{"": &representedRecord{}})
	for _, composed := range []bool{false, true} {
		provider := unsupported
		if composed {
			provider, err = locator.ComposeProviders("body", locator.ProviderLayer{Name: "unsupported", Provider: unsupported})
			require.NoError(t, err)
		}
		scope, err := injector.ForScope(provider)
		require.NoError(t, err)
		err = scope.Bind(t.Context(), &representedInput{}, WithPlan(plan))
		require.ErrorContains(t, err, "does not support body representation")
	}
}
