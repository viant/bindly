package bindly

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/viant/bindly/locator"
	"github.com/viant/bindly/state"
	"github.com/viant/structology"
)

type resolvedGroupInput struct {
	URL           string
	Rows          []int
	First, Second int
	Snapshot      string
	Has           *resolvedGroupPresence `setMarker:"true"`
}
type resolvedGroupPresence struct{ URL, Rows, First, Second, Snapshot bool }

type resolvedSnapshotTransform struct{ calls *atomic.Int32 }

func (t resolvedSnapshotTransform) Transform(_ context.Context, _ locator.Resolver, value any) (any, error) {
	t.calls.Add(1)
	return "loaded:" + value.(string), nil
}

type resolvedGroupProvider struct {
	kind    string
	resolve func(context.Context, *structology.State, string) (any, bool, error)
}

func (p resolvedGroupProvider) Kind() string  { return p.kind }
func (p resolvedGroupProvider) Priority() int { return 0 }
func (p resolvedGroupProvider) Locate(source *structology.State) locator.Locator {
	return resolvedGroupLocator{provider: p, source: source}
}

type resolvedGroupLocator struct {
	provider resolvedGroupProvider
	source   *structology.State
}

func (l resolvedGroupLocator) Kind() string { return l.provider.kind }
func (l resolvedGroupLocator) Value(ctx context.Context, _ reflect.Type, name string) (any, bool, error) {
	return l.provider.resolve(ctx, l.source, name)
}

func resolvedGroupSpecs() []BindingSpec {
	specs := []BindingSpec{
		{Path: "URL", Location: state.Location{Kind: "const", In: "url"}, Transformer: forbiddenResolvedTransform{}, SourceType: reflect.TypeFor[string]()},
		{Path: "Rows", Location: state.Location{Kind: "const", In: "rows"}},
		{Path: "Snapshot", Location: state.Location{Kind: "param", In: "URL"}},
	}
	specs = append(specs, dependencySpecs("First", "Second")...)
	for i := 3; i < len(specs); i++ {
		specs[i].ResolutionGroup.After = []string{"URL"}
	}
	return specs
}

func TestResolvedDependencyGroupOverlapAndRequestIsolation(t *testing.T) {
	ctx := dependencyContext(t)
	type event struct{ url, name string }
	entered := make(chan event, 4)
	release := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	var loaded atomic.Int32
	inj, err := NewInjector(WithProviders(
		groupProvider{"const", func(context.Context, string) (any, bool, error) {
			return nil, false, errors.New("selected seed used provider")
		}},
		resolvedGroupProvider{"component", func(ctx context.Context, source *structology.State, name string) (any, bool, error) {
			url, err := source.Value("URL")
			if err != nil {
				return nil, false, err
			}
			entered <- event{url.(string), name}
			select {
			case <-release:
				return len(url.(string)) + len(name), true, nil
			case <-ctx.Done():
				return nil, false, ctx.Err()
			}
		}},
		resolvedGroupProvider{"param", func(_ context.Context, source *structology.State, name string) (any, bool, error) {
			first, _ := source.Value("First")
			second, _ := source.Value("Second")
			if first.(int) == 0 || second.(int) == 0 {
				return nil, false, errors.New("result exposed before dependencies completed")
			}
			url, err := source.Value(name)
			return url, true, err
		}},
	))
	require.NoError(t, err)
	specs := resolvedGroupSpecs()
	specs[2].Transformer = resolvedSnapshotTransform{calls: &loaded}
	plan, err := inj.CompilePlan(reflect.TypeFor[resolvedGroupInput](), specs...)
	require.NoError(t, err)
	type result struct {
		out  *resolvedGroupInput
		seed *resolvedGroupInput
		err  error
	}
	done := make(chan result, 2)
	for _, url := range []string{"snapshot-a", "snapshot-b"} {
		go func() {
			seed := &resolvedGroupInput{URL: url, Rows: []int{7}, First: 999}
			out := &resolvedGroupInput{}
			err := inj.Bind(ctx, out, WithPlan(plan), WithSource(out), WithResolvedInput(plan, seed, "URL", "Rows"), WithResolutionGroupConcurrency(2), WithResolutionGroupController(dependencyController{}))
			done <- result{out, seed, err}
		}()
	}
	var events []event
	for range 4 {
		events = append(events, dependencyReceive(t, ctx, entered))
	}
	require.ElementsMatch(t, []event{{"snapshot-a", "First"}, {"snapshot-a", "Second"}, {"snapshot-b", "First"}, {"snapshot-b", "Second"}}, events)
	unblock()
	for range 2 {
		got := dependencyReceive(t, ctx, done)
		require.NoError(t, got.err)
		require.Equal(t, "loaded:"+got.seed.URL, got.out.Snapshot)
		require.Equal(t, len(got.seed.URL)+5, got.out.First)
		require.Equal(t, len(got.seed.URL)+6, got.out.Second)
		require.Equal(t, &resolvedGroupPresence{true, true, true, true, true}, got.out.Has)
		got.out.Rows[0] = 8
		require.Equal(t, &resolvedGroupInput{URL: got.out.URL, Rows: []int{7}, First: 999}, got.seed, "seed must remain untouched")
	}
	require.EqualValues(t, 2, loaded.Load())
}

func TestResolvedDependencyGroupRejectsInvalidPathsBeforeWork(t *testing.T) {
	type nested struct{ Value *int }
	type Embedded struct{ Value *int }
	type input struct {
		Node *nested
		Embedded
		URL   string
		Ready bool
		Has   *struct{ Value bool } `setMarker:"true"`
	}
	for _, tc := range []struct {
		name, group, seedPath, marker string
		seed                          *input
	}{
		{name: "member nil", group: "Node", seedPath: "Node", seed: &input{}},
		{name: "member populated", group: "Node", seedPath: "Node", seed: &input{Node: &nested{}}},
		{name: "ancestor nil", group: "Node.Value", seedPath: "Node", seed: &input{}},
		{name: "descendant", group: "Node", seedPath: "Node.Value", seed: &input{Node: &nested{}}},
		{name: "promoted alias", group: "Value", seedPath: "Embedded.Value", seed: &input{}},
		{name: "group marker ancestor", group: "Value", seedPath: "Has", seed: &input{}},
		{name: "group marker exact", group: "Value", seedPath: "Has.Value", seed: &input{}},
		{name: "seed marker writes group", group: "Ready", seedPath: "URL", marker: "Ready", seed: &input{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			inj, err := NewInjector(WithProviders(groupProvider{"component", func(context.Context, string) (any, bool, error) { calls.Add(1); return nil, false, nil }}))
			require.NoError(t, err)
			specs := dependencySpecs(tc.group)
			if tc.seedPath != tc.group {
				specs = append(specs, BindingSpec{Path: tc.seedPath, Location: state.Location{Kind: "const", In: "seed"}, MarkerField: tc.marker})
			}
			plan, err := inj.CompilePlan(reflect.TypeFor[input](), specs...)
			require.NoError(t, err)
			controller := &groupController{run: &groupRun{}}
			out := &input{}
			err = inj.Bind(t.Context(), out, WithPlan(plan), WithSource(out), WithResolvedInput(plan, tc.seed, tc.seedPath), WithResolutionGroupController(controller))
			require.ErrorContains(t, err, "overlaps resolution group binding")
			require.Zero(t, calls.Load())
			require.Zero(t, controller.opens)
			require.Equal(t, &input{}, out)
		})
	}
}

func TestResolvedDependencyGroupCanonicalPlanAndLegacyRestrictions(t *testing.T) {
	inj, err := NewInjector()
	require.NoError(t, err)
	for _, tc := range []struct {
		name      string
		mode      ResolutionGroupMode
		otherPlan bool
		mixed     bool
		paths     []string
	}{
		{name: "wrong plan", mode: ResolutionGroupDependencies, otherPlan: true, paths: []string{"URL"}},
		{name: "legacy", paths: []string{"URL"}},
		{name: "legacy empty selection"},
		{name: "mixed modes", mode: ResolutionGroupDependencies, mixed: true, paths: []string{"URL"}},
		{name: "duplicate", mode: ResolutionGroupDependencies, paths: []string{"URL", "URL"}},
		{name: "unknown", mode: ResolutionGroupDependencies, paths: []string{"Unknown"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			specs := resolvedGroupSpecs()
			for i := 3; i < len(specs); i++ {
				specs[i].ResolutionGroup.Mode = tc.mode
			}
			if tc.mixed {
				specs[4].ResolutionGroup.Name = "legacy"
				specs[4].ResolutionGroup.Mode = ResolutionGroupConcurrent
			}
			plan, err := inj.CompilePlan(reflect.TypeFor[resolvedGroupInput](), specs...)
			require.NoError(t, err)
			seedPlan := plan
			if tc.otherPlan {
				seedPlan, err = inj.CompilePlan(reflect.TypeFor[resolvedGroupInput](), specs...)
				require.NoError(t, err)
			}
			controller := &groupController{run: &groupRun{}}
			out := &resolvedGroupInput{}
			err = inj.Bind(t.Context(), out, WithPlan(plan), WithSource(out), WithResolvedInput(seedPlan, &resolvedGroupInput{}, tc.paths...), WithResolutionGroupController(controller))
			require.Error(t, err)
			require.Zero(t, controller.opens)
			require.Equal(t, &resolvedGroupInput{}, out)
		})
	}
}

func TestResolvedDependencyGroupRetainsInvocationRestrictions(t *testing.T) {
	for _, tc := range []struct {
		name   string
		option BindOption
	}{
		{name: "cache", option: WithValueCache(NewValueCache())},
		{name: "replay", option: WithReplay(ReplayBinding{})},
		{name: "allowed kinds", option: func(o *bindOptions) { o.allowedKinds = map[string]bool{"component": true} }},
		{name: "delayed kinds", option: func(o *bindOptions) { o.delayedKinds = map[string]bool{"param": true} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			inj, err := NewInjector(WithProviders(groupProvider{"component", func(context.Context, string) (any, bool, error) { calls.Add(1); return 1, true, nil }}))
			require.NoError(t, err)
			plan, err := inj.CompilePlan(reflect.TypeFor[resolvedGroupInput](), resolvedGroupSpecs()...)
			require.NoError(t, err)
			controller := &groupController{run: &groupRun{}}
			out := &resolvedGroupInput{}
			err = inj.Bind(t.Context(), out, WithPlan(plan), WithSource(out), WithResolvedInput(plan, &resolvedGroupInput{URL: "snapshot"}, "URL"), WithResolutionGroupController(controller), tc.option)
			require.Error(t, err)
			require.Zero(t, calls.Load())
			require.Zero(t, controller.opens)
			require.Equal(t, &resolvedGroupInput{}, out)
		})
	}
}

func TestResolvedDependencyGroupSeedValidationAndObservation(t *testing.T) {
	for _, tc := range []string{"zero", "nil", "required", "count", "observer", "condition", "false condition"} {
		t.Run(tc, func(t *testing.T) {
			var calls atomic.Int32
			inj, err := NewInjector(WithProviders(groupProvider{"component", func(context.Context, string) (any, bool, error) { calls.Add(1); return 1, true, nil }}))
			require.NoError(t, err)
			specs := resolvedGroupSpecs()
			specs = append(specs[:2], specs[3:]...)
			seed := &resolvedGroupInput{}
			specs[0].Transformer = nil
			specs[0].DefaultValue = "fallback"
			if tc == "zero" {
				seed.Rows = []int{0}
			}
			if tc == "required" {
				v := true
				specs[1].Required = &v
			}
			if tc == "count" {
				v := 1
				specs[1].MaxAllowedRecords = &v
				seed.Rows = []int{1, 2}
			}
			if tc == "condition" {
				specs[1].When = "Has.URL"
			}
			if tc == "false condition" {
				specs[1].When = "Has.Second"
				seed.Rows = []int{1}
			}
			plan, err := inj.CompilePlan(reflect.TypeFor[resolvedGroupInput](), specs...)
			require.NoError(t, err)
			out := &resolvedGroupInput{}
			controller := &groupController{run: &groupRun{}}
			var seedEvents int
			failure := errors.New("seed observer failure")
			opts := []BindOption{WithPlan(plan), WithSource(out), WithResolvedInput(plan, seed, "URL", "Rows"), WithResolutionGroupController(controller), WithBindingObserver(func(_ context.Context, event BindingEvent) error {
				if event.Path == "URL" || event.Path == "Rows" {
					seedEvents++
					if event.Metadata != nil {
						return errors.New("seed manufactured metadata")
					}
					if tc == "observer" {
						return failure
					}
				}
				return nil
			})}
			err = inj.Bind(t.Context(), out, opts...)
			if tc == "required" || tc == "count" || tc == "observer" {
				require.Error(t, err)
				require.Zero(t, controller.opens)
				require.Zero(t, calls.Load())
				if tc == "observer" {
					require.ErrorIs(t, err, failure)
				}
			} else {
				require.NoError(t, err)
				require.Equal(t, "", out.URL)
				require.True(t, out.Has.URL)
				if tc == "false condition" {
					require.Equal(t, 1, seedEvents)
					require.False(t, out.Has.Rows)
					require.Nil(t, out.Rows)
				} else {
					require.Equal(t, 2, seedEvents)
					require.True(t, out.Has.Rows)
					require.Equal(t, seed.Rows, out.Rows)
				}
				require.EqualValues(t, 2, calls.Load())
			}
		})
	}
}

func TestResolvedDependencyGroupNestedSeedWithSinglePermit(t *testing.T) {
	ctx := dependencyContext(t)
	child, err := NewInjector(WithProviders(resolvedGroupProvider{"component", func(_ context.Context, source *structology.State, _ string) (any, bool, error) {
		url, err := source.Value("URL")
		if err != nil {
			return nil, false, err
		}
		return len(url.(string)), true, nil
	}}))
	require.NoError(t, err)
	specs := append(resolvedGroupSpecs()[:1], dependencySpecs("First", "Second")...)
	childPlan, err := child.CompilePlan(reflect.TypeFor[resolvedGroupInput](), specs...)
	require.NoError(t, err)
	parent, err := NewInjector(WithProviders(resolvedGroupProvider{"component", func(ctx context.Context, source *structology.State, name string) (any, bool, error) {
		if name == "Second" {
			return 2, true, nil
		}
		url, err := source.Value("URL")
		if err != nil {
			return nil, false, err
		}
		out := &resolvedGroupInput{}
		err = SuspendResolutionGroup(ctx, func(ctx context.Context) error {
			return child.Bind(ctx, out, WithPlan(childPlan), WithSource(out), WithResolvedInput(childPlan, &resolvedGroupInput{URL: url.(string) + "-child"}, "URL"), WithResolutionGroupController(dependencyController{}))
		})
		return out.First + out.Second, true, err
	}}))
	require.NoError(t, err)
	plan, err := parent.CompilePlan(reflect.TypeFor[resolvedGroupInput](), specs...)
	require.NoError(t, err)
	out := &resolvedGroupInput{}
	err = parent.Bind(ctx, out, WithPlan(plan), WithSource(out), WithResolvedInput(plan, &resolvedGroupInput{URL: "snapshot"}, "URL"), WithResolutionGroupConcurrency(1), WithResolutionGroupController(dependencyController{}))
	require.NoError(t, err)
	require.Equal(t, 2*len("snapshot-child"), out.First)
	require.Equal(t, 2, out.Second)
}

func TestResolvedDependencyGroupErrorsRemainDeclarationOrdered(t *testing.T) {
	ctx := dependencyContext(t)
	second := make(chan struct{})
	firstErr, secondErr := errors.New("first"), errors.New("second")
	inj, err := NewInjector(WithProviders(groupProvider{"component", func(ctx context.Context, name string) (any, bool, error) {
		if name == "Second" {
			close(second)
			return nil, false, secondErr
		}
		select {
		case <-second:
			return nil, false, firstErr
		case <-ctx.Done():
			return nil, false, ctx.Err()
		}
	}}))
	require.NoError(t, err)
	specs := append(resolvedGroupSpecs()[:1], dependencySpecs("First", "Second")...)
	plan, err := inj.CompilePlan(reflect.TypeFor[resolvedGroupInput](), specs...)
	require.NoError(t, err)
	out := &resolvedGroupInput{}
	err = inj.Bind(ctx, out, WithPlan(plan), WithSource(out), WithResolvedInput(plan, &resolvedGroupInput{URL: "snapshot"}, "URL"), WithResolutionGroupController(dependencyController{}))
	var grouped *ResolutionGroupError
	require.ErrorAs(t, err, &grouped)
	require.ErrorIs(t, err, firstErr)
	require.ErrorIs(t, err, secondErr)
	require.Equal(t, "First", grouped.Failures()[0].Path)
	require.Equal(t, "Second", grouped.Failures()[1].Path)
}

func TestResolvedDependencyGroupCancellationJoinsWorkers(t *testing.T) {
	waitCtx := dependencyContext(t)
	ctx, cancel := context.WithCancel(waitCtx)
	defer cancel()
	entered := make(chan string, 2)
	var finished atomic.Int32
	inj, err := NewInjector(WithProviders(groupProvider{"component", func(ctx context.Context, name string) (any, bool, error) {
		entered <- name
		<-ctx.Done()
		finished.Add(1)
		return nil, false, ctx.Err()
	}}))
	require.NoError(t, err)
	specs := resolvedGroupSpecs()
	specs = append(specs[:2], specs[3:]...)
	plan, err := inj.CompilePlan(reflect.TypeFor[resolvedGroupInput](), specs...)
	require.NoError(t, err)
	out := &resolvedGroupInput{}
	done := make(chan error, 1)
	go func() {
		done <- inj.Bind(ctx, out, WithPlan(plan), WithSource(out), WithResolvedInput(plan, &resolvedGroupInput{URL: "snapshot"}, "URL"), WithResolutionGroupController(dependencyController{}))
	}()
	for range 2 {
		dependencyReceive(t, ctx, entered)
	}
	cancel()
	err = dependencyReceive(t, waitCtx, done)
	require.ErrorIs(t, err, context.Canceled)
	require.EqualValues(t, 2, finished.Load())
	require.Equal(t, "snapshot", out.URL)
}
