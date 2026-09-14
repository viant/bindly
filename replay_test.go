package bindly

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/viant/bindly/locator"
	"github.com/viant/bindly/provider/values"
	"github.com/viant/bindly/state"
	"github.com/viant/structology"
	"reflect"
	"strings"
	"testing"
)

type replayEvidenceProvider struct {
	calls *int
	value int
}

func (p replayEvidenceProvider) Kind() string                              { return "dependency" }
func (p replayEvidenceProvider) Priority() int                             { return 20 }
func (p replayEvidenceProvider) Locate(*structology.State) locator.Locator { return p }
func (p replayEvidenceProvider) Value(context.Context, reflect.Type, string) (any, bool, error) {
	*p.calls++
	return locator.ValueWithMetadata{Value: p.value, Metadata: "fresh-read"}, true, nil
}

type replayCodec struct{ calls *int }

func (c replayCodec) Transform(_ context.Context, _ locator.Resolver, value any) (any, error) {
	*c.calls++
	token, ok := value.(string)
	if !ok || token != "signed" {
		return nil, errors.New("verify source")
	}
	return 7, nil
}

func TestReplayBindsExternalThenFreshDependencies(t *testing.T) {
	type input struct {
		ID, Current, Claim int
		Has                struct{ ID, Current, Claim bool } `setMarker:"true"`
	}
	calls, codecCalls := 0, 0
	injector, err := NewInjector(WithProviders(replayEvidenceProvider{calls: &calls, value: 42}, values.New("query", map[string]any{"id": 999})))
	if err != nil {
		t.Fatal(err)
	}
	plan, err := injector.CompilePlan(reflect.TypeOf(input{}), BindingSpec{Path: "Current", Name: "Current", Location: state.Location{Kind: "dependency"}}, BindingSpec{Path: "ID", Name: "ID", Location: state.Location{Kind: "query", In: "id"}}, BindingSpec{Path: "Claim", Name: "Claim", Location: state.Location{Kind: "header", In: "Authorization"}, SourceType: reflect.TypeOf(""), Transformer: replayCodec{&codecCalls}})
	if err != nil {
		t.Fatal(err)
	}
	replayPlan, err := plan.Replay("ID", "Claim")
	if err != nil {
		t.Fatal(err)
	}
	replay, err := replayPlan.DecodeJSON([]byte(`{"ID":0,"Claim":"signed","Current":999}`))
	if err != nil {
		t.Fatal(err)
	}
	prepared := &input{}
	if err := injector.Bind(context.Background(), prepared, WithPlan(plan), WithReplay(ReplayBinding{Replay: replay, Only: true, Gate: func(_ context.Context, target any) error {
		actual := target.(*input)
		if actual.ID != 0 || !actual.Has.ID || actual.Claim != 7 || actual.Current != 0 || calls != 0 {
			t.Fatalf("preflight=%+v reads=%d", actual, calls)
		}
		return nil
	}})); err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(replay)
	if string(encoded) != `{"Claim":"signed","ID":0}` {
		t.Fatalf("persisted codec/dependency value: %s", encoded)
	}
	prepared.ID = 99 // Cannot alias the native prepared seed.
	actual := &input{}
	events := map[string]any{}
	if err := injector.Bind(context.Background(), actual, WithPlan(plan), WithReplay(ReplayBinding{Replay: replay}), WithBindingObserver(func(_ context.Context, event BindingEvent) error { events[event.Path] = event.Metadata; return nil })); err != nil {
		t.Fatal(err)
	}
	if actual.ID != 0 || actual.Current != 42 || actual.Claim != 7 || calls != 1 || codecCalls != 1 {
		t.Fatalf("result=%+v reads=%d codec calls=%d", actual, calls, codecCalls)
	}
	if events["Current"] != "fresh-read" || events["ID"] != nil || events["Claim"] != nil {
		t.Fatalf("fabricated/stale metadata: %v", events)
	}
}
func TestReplayRejectsDecodedCodecAndPreservesAbsence(t *testing.T) {
	type input struct {
		Claim int
		ID    int
		Has   struct{ Claim, ID bool } `setMarker:"true"`
	}
	calls := 0
	i, _ := NewInjector()
	p, err := i.CompilePlan(reflect.TypeOf(input{}), BindingSpec{Path: "Claim", Name: "Claim", Location: state.Location{Kind: "header", In: "Authorization"}, SourceType: reflect.TypeOf(""), Transformer: replayCodec{&calls}}, BindingSpec{Path: "ID", Name: "ID", Location: state.Location{Kind: "query", In: "id"}})
	if err != nil {
		t.Fatal(err)
	}
	rp, _ := p.Replay("Claim", "ID")
	for _, raw := range []string{`{"Claim":7}`, `{"Claim":{"sub":"forged"}}`} {
		replay, _ := rp.DecodeJSON([]byte(raw))
		if err := i.Bind(context.Background(), &input{}, WithPlan(p), WithReplay(ReplayBinding{Replay: replay, Only: true})); err == nil {
			t.Fatal("decoded claims accepted")
		}
	}
	if calls != 0 {
		t.Fatal("invalid source reached verifier")
	}
	replay, _ := rp.DecodeJSON([]byte(`{}`))
	providers, err := replay.Providers()
	if err != nil {
		t.Fatal(err)
	}
	query := providers[1]
	if query.Kind() != "query" {
		t.Fatal(query.Kind())
	}
	layered, err := locator.ComposeProviders("query", locator.ProviderLayer{Name: "replay", Provider: query}, locator.ProviderLayer{Name: "fallback", Provider: values.New("query", map[string]any{"id": 999})})
	if err != nil {
		t.Fatal(err)
	}
	scoped, _ := i.ForScope(layered)
	actual := &input{}
	if err := scoped.Bind(context.Background(), actual, WithPlan(p), WithReplay(ReplayBinding{Replay: replay})); err != nil {
		t.Fatal(err)
	}
	if actual.Has.ID || actual.ID != 0 {
		t.Fatalf("absence fell through: %+v", actual)
	}
	nativeOnly, _ := scoped.ForScope()
	var target struct {
		ID int `bind:"kind=query,in=id"`
	}
	if err := nativeOnly.Bind(context.Background(), &target); err != nil || target.ID != 0 {
		t.Fatalf("dependency fallback: %+v %v", target, err)
	}
}
func TestReplayBodyPresenceUsesNativeBodyOwner(t *testing.T) {
	type row struct {
		ID   int                      `json:"id,omitempty"`
		Name string                   `json:"name,omitempty"`
		Has  *struct{ ID, Name bool } `json:"-" setMarker:"true"`
	}
	type input struct {
		Rows []*row
		Has  struct{ Rows bool } `setMarker:"true"`
	}
	i, _ := NewInjector()
	p, err := i.CompilePlan(reflect.TypeOf(input{}), BindingSpec{Name: "Rows", Path: "Rows", Location: state.Location{Kind: "body", In: "Data"}})
	if err != nil {
		t.Fatal(err)
	}
	rp, _ := p.Replay("Rows")
	replay, _ := rp.DecodeJSON([]byte(`{"Rows":[{"id":0},{"name":"b"}]}`))
	actual := &input{}
	if err := i.Bind(context.Background(), actual, WithPlan(p), WithReplay(ReplayBinding{Replay: replay, Only: true})); err != nil {
		t.Fatal(err)
	}
	if !actual.Rows[0].Has.ID || actual.Rows[0].Has.Name || actual.Rows[1].Has.ID || !actual.Rows[1].Has.Name {
		t.Fatal("body presence lost")
	}
	if _, err := rp.Capture(actual); err == nil || !strings.Contains(err.Error(), "raw source") {
		t.Fatalf("lossy typed capture accepted: %v", err)
	}
	encoded, _ := json.Marshal(replay)
	if string(encoded) != `{"Rows":[{"id":0},{"name":"b"}]}` {
		t.Fatalf("raw zero lost: %s", encoded)
	}
}

func TestReplayGateCannotReplaceCodecProof(t *testing.T) {
	type input struct{ Claim int }
	calls := 0
	i, _ := NewInjector()
	plan, err := i.CompilePlan(reflect.TypeOf(input{}), BindingSpec{Path: "Claim", Name: "Claim", Location: state.Location{Kind: "header", In: "Authorization"}, SourceType: reflect.TypeOf(""), Transformer: replayCodec{&calls}})
	if err != nil {
		t.Fatal(err)
	}
	rp, _ := plan.Replay("Claim")
	replay, _ := rp.DecodeJSON([]byte(`{"Claim":"signed"}`))
	err = i.Bind(context.Background(), &input{}, WithPlan(plan), WithReplay(ReplayBinding{Replay: replay, Only: true, Gate: func(_ context.Context, target any) error { target.(*input).Claim = 99; return nil }}))
	if err == nil || replay.prepared != nil {
		t.Fatal("modified codec output became prepared proof")
	}
}
func TestReplayAbsentWithoutMarkerStaysAbsentForDependencies(t *testing.T) {
	type input struct{ ID int }
	i, _ := NewInjector()
	plan, _ := i.CompilePlan(reflect.TypeOf(input{}), BindingSpec{Path: "ID", Name: "ID", Location: state.Location{Kind: "query", In: "id"}})
	rp, _ := plan.Replay("ID")
	replay, _ := rp.DecodeJSON([]byte(`{}`))
	if err := i.Bind(context.Background(), &input{}, WithPlan(plan), WithReplay(ReplayBinding{Replay: replay, Only: true})); err != nil {
		t.Fatal(err)
	}
	providers, _ := replay.Providers()
	scope, _ := i.ForScope(providers...)
	target := &struct {
		ID  int               `bind:"kind=query,in=id"`
		Has struct{ ID bool } `setMarker:"true"`
	}{}
	if err := scope.Bind(context.Background(), target); err != nil {
		t.Fatal(err)
	}
	if target.Has.ID {
		t.Fatal("missing source was promoted to present zero")
	}
}
