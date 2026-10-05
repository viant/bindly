package bindly

import (
	"context"
	"errors"
	"github.com/viant/bindly/locator"
	"github.com/viant/bindly/state"
	"reflect"
	"strings"
	"sync"
	"testing"
)

type resolvedMarkers struct{ Value, Rows bool }
type resolvedRecord struct {
	Value   int
	Rows    []int
	Enabled bool
	Has     *resolvedMarkers `setMarker:"true"`
}
type forbiddenResolvedTransform struct{}

func (forbiddenResolvedTransform) Transform(context.Context, locator.Resolver, any) (any, error) {
	return nil, errors.New("transformer must not run")
}

func resolvedPlan(t *testing.T, inj *Injector, when string, required bool, max *int) *Plan {
	t.Helper()
	p, e := inj.CompilePlan(reflect.TypeFor[resolvedRecord](), BindingSpec{Path: "Value", Location: state.Location{Kind: "metadata", In: "value"}, Transformer: forbiddenResolvedTransform{}, SourceType: reflect.TypeFor[string](), When: when}, BindingSpec{Path: "Rows", Location: state.Location{Kind: "metadata", In: "rows"}, Required: &required, MaxAllowedRecords: max})
	if e != nil {
		t.Fatal(e)
	}
	return p
}
func TestResolvedInputCanonicalAssignmentAndDetachment(t *testing.T) {
	provider := &metadataProvider{err: errors.New("provider must not run")}
	inj, _ := NewInjector(WithProviders(provider))
	plan := resolvedPlan(t, inj, "", false, nil)
	seed := &resolvedRecord{Value: 0, Rows: []int{4}}
	out := &resolvedRecord{}
	var events []BindingEvent
	err := inj.Bind(context.Background(), out, WithPlan(plan), WithSource(out), WithResolvedInput(plan, seed, "Value", "Rows"), WithBindingObserver(func(_ context.Context, e BindingEvent) error { events = append(events, e); return nil }))
	if err != nil {
		t.Fatal(err)
	}
	if out.Has == nil || !out.Has.Value || !out.Has.Rows || out.Value != 0 || len(events) != 2 || provider.calls != 0 {
		t.Fatalf("assignment/marker/observer mismatch")
	}
	for _, e := range events {
		if e.Metadata != nil {
			t.Fatal("seed invented provenance")
		}
	}
	seed.Rows[0] = 8
	if out.Rows[0] != 4 {
		t.Fatal("seed aliases destination")
	}
	out.Rows[0] = 9
	if seed.Rows[0] != 8 {
		t.Fatal("destination aliases seed")
	}
}
func TestResolvedInputConditionAndUnselectedBindings(t *testing.T) {
	provider := &metadataProvider{value: []int{3}, found: true}
	inj, _ := NewInjector(WithProviders(provider))
	plan := resolvedPlan(t, inj, "Enabled", false, nil)
	out := &resolvedRecord{Enabled: false}
	if err := inj.Bind(context.Background(), out, WithPlan(plan), WithSource(out), WithResolvedInput(plan, &resolvedRecord{Value: 17}, "Value")); err != nil {
		t.Fatal(err)
	}
	if out.Value != 0 || out.Has == nil || out.Has.Value || !out.Has.Rows || provider.calls != 1 || out.Rows[0] != 3 {
		t.Fatal("condition skipped or unrelated binding bypassed")
	}
}
func TestResolvedInputRequiredNullAndRecordCounts(t *testing.T) {
	for _, tc := range []struct {
		name     string
		required bool
		rows     []int
		max      *int
		wantErr  bool
	}{
		{name: "optional null"}, {name: "required null", required: true, wantErr: true},
		{name: "required empty", required: true, rows: []int{}, wantErr: true},
		{name: "count too high", rows: []int{1, 2}, max: func() *int { x := 1; return &x }(), wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			inj, _ := NewInjector()
			plan := resolvedPlan(t, inj, "", tc.required, tc.max)
			out := &resolvedRecord{}
			err := inj.Bind(context.Background(), out, WithPlan(plan), WithResolvedInput(plan, &resolvedRecord{Rows: tc.rows}, "Value", "Rows"))
			if (err != nil) != tc.wantErr {
				t.Fatalf("error=%v", err)
			}
			if !tc.wantErr && (out.Has == nil || !out.Has.Rows || out.Rows != nil) {
				t.Fatal("optional present null lost")
			}
		})
	}
}
func TestResolvedInputRejectsInvalidAuthorityBeforeBinding(t *testing.T) {
	provider := &metadataProvider{err: errors.New("binding executed before validation")}
	inj, _ := NewInjector(WithProviders(provider))
	plan := resolvedPlan(t, inj, "", false, nil)
	other := resolvedPlan(t, inj, "", false, nil)
	for _, tc := range []struct {
		name     string
		seed     any
		paths    []string
		plan     *Plan
		replay   bool
		repeat   bool
		omitPlan bool
	}{
		{name: "wrong canonical plan", seed: &resolvedRecord{}, plan: other},
		{name: "nil canonical plan", seed: &resolvedRecord{}},
		{name: "nil input", plan: plan}, {name: "typed nil", seed: (*resolvedRecord)(nil), plan: plan},
		{name: "wrong type", seed: &struct{ Value int }{}, plan: plan},
		{name: "unknown path", seed: &resolvedRecord{}, plan: plan, paths: []string{"Missing"}},
		{name: "marker is not a binding", seed: &resolvedRecord{}, plan: plan, paths: []string{"Has.Value"}},
		{name: "duplicate path", seed: &resolvedRecord{}, plan: plan, paths: []string{"Value", "Value"}},
		{name: "replay", seed: &resolvedRecord{}, plan: plan, replay: true},
		{name: "repeat option", seed: &resolvedRecord{}, plan: plan, repeat: true},
		{name: "omit WithPlan", seed: &resolvedRecord{}, plan: plan, omitPlan: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := &resolvedRecord{}
			opts := []BindOption{WithPlan(plan), WithResolvedInput(tc.plan, tc.seed, tc.paths...)}
			if tc.omitPlan {
				opts = opts[1:]
			}
			if tc.replay {
				opts = append(opts, WithReplay(ReplayBinding{}))
			}
			if tc.repeat {
				opts = append(opts, WithResolvedInput(plan, &resolvedRecord{}))
			}
			err := inj.Bind(context.Background(), out, opts...)
			if err == nil || provider.calls != 0 || out.Has != nil {
				t.Fatalf("invalid seed entered binding: %v", err)
			}
		})
	}
}
func TestResolvedInputObserverFailureRemainsBindingError(t *testing.T) {
	inj, _ := NewInjector()
	plan := resolvedPlan(t, inj, "", false, nil)
	out := &resolvedRecord{}
	err := inj.Bind(context.Background(), out, WithPlan(plan), WithResolvedInput(plan, &resolvedRecord{Value: 5}, "Value", "Rows"), WithBindingObserver(func(context.Context, BindingEvent) error { return errors.New("observer failure") }))
	var failure *BindingError
	if !errors.As(err, &failure) || failure.Path != "Value" || !strings.Contains(err.Error(), "observer failure") || out.Value != 5 || out.Has == nil || !out.Has.Value {
		t.Fatalf("assignment/observer error ordering changed: %v", err)
	}
}
func TestResolvedInputConcurrentBindingsOwnValues(t *testing.T) {
	inj, _ := NewInjector()
	plan := resolvedPlan(t, inj, "", false, nil)
	seed := &resolvedRecord{Value: 7, Rows: []int{4}}
	var wg sync.WaitGroup
	for n := 0; n < 40; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out := &resolvedRecord{}
			if err := inj.Bind(context.Background(), out, WithPlan(plan), WithSource(out), WithResolvedInput(plan, seed, "Value", "Rows")); err != nil {
				t.Error(err)
				return
			}
			out.Rows[0] = 99
		}()
	}
	wg.Wait()
	if seed.Rows[0] != 4 {
		t.Fatal("concurrent child changed supplied seed")
	}
}

func TestResolvedInputPreservesSelectedAliases(t *testing.T) {
	type item struct{ ID int }
	type input struct{ A, B *item }
	inj, _ := NewInjector()
	plan, err := inj.CompilePlan(reflect.TypeFor[input](), BindingSpec{Path: "A", Location: state.Location{Kind: "metadata", In: "a"}}, BindingSpec{Path: "B", Location: state.Location{Kind: "metadata", In: "b"}})
	if err != nil {
		t.Fatal(err)
	}
	shared := &item{ID: 7}
	seed := &input{A: shared, B: shared}
	out := &input{}
	if err := inj.Bind(context.Background(), out, WithPlan(plan), WithResolvedInput(plan, seed, "A", "B")); err != nil {
		t.Fatal(err)
	}
	if out.A != out.B || out.A == shared {
		t.Fatal("selected alias identity or detachment lost")
	}
}
