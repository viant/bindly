package bindly

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/viant/bindly/locator"
	"github.com/viant/bindly/state"
	"github.com/viant/structology"
)

type dependencyInput struct {
	First, Second, Third int
	Has                  *dependencyPresence `setMarker:"true"`
}
type dependencyPresence struct{ First, Second, Third bool }

func dependencySpecs(names ...string) []BindingSpec {
	var result []BindingSpec
	for _, name := range names {
		result = append(result, BindingSpec{Path: name, Location: state.Location{Kind: "component", In: name}, ResolutionGroup: &ResolutionGroupSpec{Name: "reads", Mode: ResolutionGroupDependencies}})
	}
	return result
}

type dependencyController struct {
	open func(ResolutionGroupPlan) (ResolutionGroupRun, error)
}

func (c dependencyController) Open(_ context.Context, plan ResolutionGroupPlan) (ResolutionGroupRun, error) {
	if c.open != nil {
		return c.open(plan)
	}
	return &groupRun{}, nil
}

func dependencyContext(t *testing.T) context.Context {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func dependencyReceive[T any](t *testing.T, ctx context.Context, ch <-chan T) T {
	t.Helper()
	select {
	case value := <-ch:
		return value
	case <-ctx.Done():
		t.Fatalf("waiting for synchronized event: %v", ctx.Err())
		var zero T
		return zero
	}
}

func TestDependencyGroupBoundsAdmissionAndReusesReleasedCapacity(t *testing.T) {
	ctx := dependencyContext(t)
	entered := make(chan string, 3)
	releases := map[string]chan struct{}{"First": make(chan struct{}), "Second": make(chan struct{}), "Third": make(chan struct{})}
	var active, peak atomic.Int32
	injector, err := NewInjector(WithProviders(groupProvider{"component", func(ctx context.Context, name string) (any, bool, error) {
		current := active.Add(1)
		defer active.Add(-1)
		for old := peak.Load(); old < current && !peak.CompareAndSwap(old, current); old = peak.Load() {
		}
		entered <- name
		select {
		case <-releases[name]:
			return len(name), true, nil
		case <-ctx.Done():
			return nil, false, ctx.Err()
		}
	}}))
	require.NoError(t, err)
	plan, err := injector.CompilePlan(reflect.TypeFor[dependencyInput](), dependencySpecs("First", "Second", "Third")...)
	require.NoError(t, err)
	input := &dependencyInput{}
	run := &groupRun{observed: make(chan string, 3)}
	done := make(chan error, 1)
	go func() {
		done <- injector.Bind(ctx, input, WithPlan(plan), WithSource(input), WithResolutionGroupConcurrency(2), WithResolutionGroupController(dependencyController{open: func(plan ResolutionGroupPlan) (ResolutionGroupRun, error) {
			if plan.Mode() != ResolutionGroupDependencies || len(plan.After()) != 0 {
				return nil, errors.New("incorrect canonical mode/barriers")
			}
			return run, nil
		}}))
	}()
	first := dependencyReceive(t, ctx, entered)
	second := dependencyReceive(t, ctx, entered)
	require.ElementsMatch(t, []string{"First", "Second"}, []string{first, second})
	select {
	case name := <-entered:
		t.Fatalf("exceeded concurrency limit: %s", name)
	default:
	}
	close(releases["First"])
	require.Equal(t, "First", dependencyReceive(t, ctx, run.observed))
	require.Equal(t, "Third", dependencyReceive(t, ctx, entered))
	// Second is still blocked; a released slot admits pending work immediately.
	close(releases["Third"])
	close(releases["Second"])
	require.NoError(t, dependencyReceive(t, ctx, done))
	require.EqualValues(t, 2, peak.Load())
	require.Equal(t, &dependencyInput{First: 5, Second: 6, Third: 5, Has: &dependencyPresence{true, true, true}}, input)
	require.True(t, run.closed)
}

func TestDependencyGroupPrerequisitesWaitForCompleteProvider(t *testing.T) {
	ctx := dependencyContext(t)
	entered := make(chan string, 3)
	releaseFinalizer := make(chan struct{})
	var finalized atomic.Bool
	injector, err := NewInjector(WithProviders(groupProvider{"component", func(ctx context.Context, name string) (any, bool, error) {
		entered <- name
		if name == "First" {
			select {
			case <-releaseFinalizer:
				finalized.Store(true)
			case <-ctx.Done():
				return nil, false, ctx.Err()
			}
		} else if !finalized.Load() {
			return nil, false, errors.New("prerequisite exposed before finalizer")
		}
		return 7, true, nil
	}}))
	require.NoError(t, err)
	// Deliberately declare dependents before their prerequisites.
	specs := dependencySpecs("Third", "Second", "First")
	specs[0].ResolutionGroup.DependsOn = []string{"Second"}
	specs[1].ResolutionGroup.DependsOn = []string{"First"}
	plan, err := injector.CompilePlan(reflect.TypeFor[dependencyInput](), specs...)
	require.NoError(t, err)
	input := &dependencyInput{}
	done := make(chan error, 1)
	go func() {
		done <- injector.Bind(ctx, input, WithPlan(plan), WithSource(input), WithResolutionGroupController(dependencyController{}))
	}()
	require.Equal(t, "First", dependencyReceive(t, ctx, entered))
	select {
	case name := <-entered:
		t.Fatalf("dependent started before provider completion: %s", name)
	default:
	}
	close(releaseFinalizer)
	require.Equal(t, "Second", dependencyReceive(t, ctx, entered))
	require.Equal(t, "Third", dependencyReceive(t, ctx, entered))
	require.NoError(t, dependencyReceive(t, ctx, done))
	require.Equal(t, 7, input.Third)
}

func TestDependencyGroupErrorsFollowDeclarationNotCompletion(t *testing.T) {
	ctx := dependencyContext(t)
	entered := make(chan string, 3)
	releases := map[string]chan struct{}{}
	causes := map[string]error{}
	for _, name := range []string{"First", "Second", "Third"} {
		releases[name], causes[name] = make(chan struct{}), errors.New(name)
	}
	injector, err := NewInjector(WithProviders(groupProvider{"component", func(ctx context.Context, name string) (any, bool, error) {
		entered <- name
		select {
		case <-releases[name]:
			return nil, false, causes[name]
		case <-ctx.Done():
			return nil, false, ctx.Err()
		}
	}}))
	require.NoError(t, err)
	plan, err := injector.CompilePlan(reflect.TypeFor[dependencyInput](), dependencySpecs("First", "Second", "Third")...)
	require.NoError(t, err)
	run := &groupRun{observed: make(chan string, 3)}
	done := make(chan error, 1)
	input := &dependencyInput{}
	go func() {
		done <- injector.Bind(ctx, input, WithPlan(plan), WithSource(input), WithResolutionGroupController(&groupController{run: run}))
	}()
	for range 3 {
		dependencyReceive(t, ctx, entered)
	}
	for _, name := range []string{"Third", "Second", "First"} {
		close(releases[name])
		require.Equal(t, name, dependencyReceive(t, ctx, run.observed))
	}
	err = dependencyReceive(t, ctx, done)
	var grouped *ResolutionGroupError
	require.ErrorAs(t, err, &grouped)
	for i, name := range []string{"First", "Second", "Third"} {
		require.Equal(t, name, grouped.Failures()[i].Path)
		require.ErrorIs(t, err, causes[name])
	}
	require.Nil(t, input.Has)
}

func TestDependencyGroupFailedPrerequisitesAreSkippedExactlyOnce(t *testing.T) {
	cause := errors.New("failed root")
	var calls atomic.Int32
	injector, err := NewInjector(WithProviders(groupProvider{"component", func(context.Context, string) (any, bool, error) {
		calls.Add(1)
		return nil, false, cause
	}}))
	require.NoError(t, err)
	specs := dependencySpecs("Third", "Second", "First")
	specs[0].ResolutionGroup.DependsOn = []string{"Second"}
	specs[1].ResolutionGroup.DependsOn = []string{"First"}
	plan, err := injector.CompilePlan(reflect.TypeFor[dependencyInput](), specs...)
	require.NoError(t, err)
	run := &groupRun{}
	input := &dependencyInput{}
	err = injector.Bind(dependencyContext(t), input, WithPlan(plan), WithSource(input), WithResolutionGroupController(&groupController{run: run}))
	require.ErrorIs(t, err, cause)
	require.EqualValues(t, 1, calls.Load())
	require.Len(t, run.outcomes, 3)
	for _, outcome := range run.outcomes {
		if outcome.Path() == "First" {
			require.True(t, outcome.Attempted())
		} else {
			require.False(t, outcome.Attempted())
			require.Nil(t, outcome.Failure())
			var skipped *ResolutionPrerequisiteError
			require.ErrorAs(t, outcome.Cause(), &skipped)
		}
	}
	require.True(t, run.closed)
}

func TestDependencyGroupNestedSuspensionAllowsPendingSibling(t *testing.T) {
	for _, limit := range []int{1, 2} {
		t.Run(fmt.Sprint(limit), func(t *testing.T) {
			ctx := dependencyContext(t)
			parentEntered := make(chan struct{})
			siblingEntered := make(chan struct{})
			allowDescendants := make(chan struct{})
			siblingRelease := make(chan struct{})
			var childComplete atomic.Bool
			childInjector, err := NewInjector(WithProviders(groupProvider{"component", func(context.Context, string) (any, bool, error) { return 9, true, nil }}))
			require.NoError(t, err)
			childPlan, err := childInjector.CompilePlan(reflect.TypeFor[dependencyInput](), dependencySpecs("First", "Second")...)
			require.NoError(t, err)
			injector, err := NewInjector(WithProviders(groupProvider{"component", func(ctx context.Context, name string) (any, bool, error) {
				if name == "Second" {
					close(siblingEntered)
					select {
					case <-siblingRelease:
						return 2, true, nil
					case <-ctx.Done():
						return nil, false, ctx.Err()
					}
				}
				close(parentEntered)
				err := SuspendResolutionGroup(ctx, func(ctx context.Context) error {
					select {
					case <-allowDescendants:
					case <-ctx.Done():
						return ctx.Err()
					}
					child := &dependencyInput{}
					if err := childInjector.Bind(ctx, child, WithPlan(childPlan), WithSource(child), WithResolutionGroupController(dependencyController{})); err != nil {
						return err
					}
					if child.First != 9 || child.Second != 9 {
						return errors.New("incomplete nested output")
					}
					childComplete.Store(true)
					return nil
				})
				if err != nil {
					return nil, false, err
				}
				return 1, true, nil
			}}))
			require.NoError(t, err)
			plan, err := injector.CompilePlan(reflect.TypeFor[dependencyInput](), dependencySpecs("First", "Second")...)
			require.NoError(t, err)
			input := &dependencyInput{}
			done := make(chan error, 1)
			go func() {
				done <- injector.Bind(ctx, input, WithPlan(plan), WithSource(input), WithResolutionGroupConcurrency(limit), WithResolutionGroupController(dependencyController{}))
			}()
			dependencyReceive(t, ctx, parentEntered)
			dependencyReceive(t, ctx, siblingEntered)
			require.False(t, childComplete.Load())
			// The sibling has started while the original parent is still suspended.
			close(siblingRelease)
			close(allowDescendants)
			require.NoError(t, dependencyReceive(t, ctx, done))
			require.True(t, childComplete.Load())
			require.Equal(t, 1, input.First)
			require.Equal(t, 2, input.Second)
		})
	}
}

func TestDependencyGroupCancellationAndPanicJoinStartedWork(t *testing.T) {
	for _, panicProvider := range []bool{false, true} {
		t.Run(fmt.Sprint(panicProvider), func(t *testing.T) {
			baseCtx := dependencyContext(t)
			ctx, cancel := context.WithCancel(baseCtx)
			defer cancel()
			entered := make(chan string, 3)
			trigger := make(chan struct{})
			cleanupStarted := make(chan struct{})
			cleanupRelease := make(chan struct{})
			cause := errors.New("provider panic")
			injector, err := NewInjector(WithProviders(groupProvider{"component", func(ctx context.Context, name string) (any, bool, error) {
				entered <- name
				if name == "First" {
					select {
					case <-trigger:
						panic(cause)
					case <-ctx.Done():
						return nil, false, ctx.Err()
					}
				}
				<-ctx.Done()
				close(cleanupStarted)
				<-cleanupRelease
				return nil, false, ctx.Err()
			}}))
			require.NoError(t, err)
			plan, err := injector.CompilePlan(reflect.TypeFor[dependencyInput](), dependencySpecs("First", "Second", "Third")...)
			require.NoError(t, err)
			run := &groupRun{}
			input := &dependencyInput{}
			done := make(chan error, 1)
			go func() {
				done <- injector.Bind(ctx, input, WithPlan(plan), WithSource(input), WithResolutionGroupConcurrency(2), WithResolutionGroupController(&groupController{run: run}))
			}()
			dependencyReceive(t, ctx, entered)
			dependencyReceive(t, ctx, entered)
			if panicProvider {
				close(trigger)
			} else {
				cancel()
			}
			dependencyReceive(t, baseCtx, cleanupStarted)
			select {
			case err := <-done:
				t.Fatalf("returned before joining cleanup: %v", err)
			default:
			}
			close(cleanupRelease)
			err = dependencyReceive(t, baseCtx, done)
			require.Error(t, err)
			if panicProvider {
				require.ErrorIs(t, err, cause)
			} else {
				require.ErrorIs(t, err, context.Canceled)
			}
			require.True(t, run.closed)
			require.Len(t, run.outcomes, 3)
			for _, outcome := range run.outcomes {
				if outcome.Path() == "Third" {
					require.False(t, outcome.Attempted())
				}
			}
		})
	}
}

func TestDependencyGroupOptionalAbsenceAndErrors(t *testing.T) {
	cause := errors.New("optional provider failed")
	injector, err := NewInjector(WithProviders(groupProvider{"component", func(_ context.Context, name string) (any, bool, error) {
		switch name {
		case "First":
			return nil, false, nil
		case "Second":
			return 0, true, nil
		default:
			return nil, false, cause
		}
	}}))
	require.NoError(t, err)
	plan, err := injector.CompilePlan(reflect.TypeFor[dependencyInput](), dependencySpecs("First", "Second", "Third")...)
	require.NoError(t, err)
	input := &dependencyInput{}
	err = injector.Bind(dependencyContext(t), input, WithPlan(plan), WithSource(input), WithResolutionGroupController(dependencyController{}))
	require.ErrorIs(t, err, cause)
	require.Equal(t, &dependencyPresence{false, true, false}, input.Has)
}

func TestDependencyGroupConcurrentInvocationsRemainIsolated(t *testing.T) {
	injector, err := NewInjector()
	require.NoError(t, err)
	plan, err := injector.CompilePlan(reflect.TypeFor[dependencyInput](), dependencySpecs("First", "Second", "Third")...)
	require.NoError(t, err)
	var wait sync.WaitGroup
	for id := 1; id <= 20; id++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			scoped, err := injector.ForScope(groupProvider{"component", func(context.Context, string) (any, bool, error) { return id, true, nil }})
			if err != nil {
				t.Error(err)
				return
			}
			input := &dependencyInput{}
			err = scoped.Bind(t.Context(), input, WithPlan(plan), WithSource(input), WithResolutionGroupConcurrency(2), WithResolutionGroupController(dependencyController{}), WithBindingObserver(func(_ context.Context, event BindingEvent) error {
				// Reading the canonical record during observation must not race publication.
				if input.First != 0 && input.First != id || input.Second != 0 && input.Second != id || input.Third != 0 && input.Third != id || event.Target != input {
					return errors.New("cross-request state contamination")
				}
				return nil
			}))
			if err != nil || input.First != id || input.Second != id || input.Third != id {
				t.Errorf("request %d: %+v, %v", id, input, err)
			}
		}()
	}
	wait.Wait()
}

func TestDependencyGroupValidationAndTagMode(t *testing.T) {
	injector, err := NewInjector()
	require.NoError(t, err)
	for _, test := range []struct {
		name string
		edit func([]BindingSpec)
	}{
		{"unknown prerequisite", func(s []BindingSpec) { s[0].ResolutionGroup.DependsOn = []string{"Missing"} }},
		{"cycle", func(s []BindingSpec) {
			s[0].ResolutionGroup.DependsOn = []string{"Second"}
			s[1].ResolutionGroup.DependsOn = []string{"First"}
		}},
		{"unknown mode", func(s []BindingSpec) { s[0].ResolutionGroup.Mode = "unknown" }},
		{"mixed modes", func(s []BindingSpec) {
			s[0].ResolutionGroup.Mode = ResolutionGroupConcurrent
			s[0].ResolutionGroup.After = []string{"Third"}
		}},
		{"legacy without barriers", func(s []BindingSpec) { s[0].ResolutionGroup.Mode = ResolutionGroupConcurrent }},
	} {
		t.Run(test.name, func(t *testing.T) {
			specs := dependencySpecs("First", "Second")
			specs = append(specs, BindingSpec{Path: "Third", Location: state.Location{Kind: "query", In: "third"}})
			test.edit(specs)
			_, err := injector.CompilePlan(reflect.TypeFor[dependencyInput](), specs...)
			require.Error(t, err)
		})
	}
	type tagged struct {
		First int `bind:"kind=component,in=First,resolutionGroup=reads,resolutionMode=dependencies"`
	}
	_, err = injector.CompilePlan(reflect.TypeFor[tagged]())
	require.NoError(t, err)
	require.ErrorContains(t, injector.Bind(t.Context(), &dependencyInput{}, WithResolutionGroupConcurrency(-1)), "negative")
}

func TestDependencyGroupSaturatedRecursiveBranches(t *testing.T) {
	for _, limit := range []int{1, 2} {
		t.Run(fmt.Sprint(limit), func(t *testing.T) {
			ctx := dependencyContext(t)
			var active, peak atomic.Int32
			enter := func() {
				current := active.Add(1)
				for old := peak.Load(); old < current && !peak.CompareAndSwap(old, current); old = peak.Load() {
				}
			}
			var build func(int) (*Injector, *Plan)
			build = func(depth int) (*Injector, *Plan) {
				var childInjector *Injector
				var childPlan *Plan
				if depth > 0 {
					childInjector, childPlan = build(depth - 1)
				}
				injector, err := NewInjector(WithProviders(groupProvider{"component", func(ctx context.Context, _ string) (any, bool, error) {
					enter()
					if depth == 0 {
						active.Add(-1)
						return 1, true, nil
					}
					child := &dependencyInput{}
					active.Add(-1)
					err := SuspendResolutionGroup(ctx, func(ctx context.Context) error {
						// Sequential descendant phases use the same budget too.
						if err := RunResolutionGroupPhase(ctx, func(context.Context) error {
							enter()
							active.Add(-1)
							return nil
						}); err != nil {
							return err
						}
						return childInjector.Bind(ctx, child, WithPlan(childPlan), WithSource(child), WithResolutionGroupController(dependencyController{}))
					})
					if err != nil {
						return nil, false, err
					}
					enter()
					value := child.First + child.Second
					active.Add(-1)
					return value, true, nil
				}}))
				require.NoError(t, err)
				plan, err := injector.CompilePlan(reflect.TypeFor[dependencyInput](), dependencySpecs("First", "Second")...)
				require.NoError(t, err)
				return injector, plan
			}
			injector, plan := build(3)
			input := &dependencyInput{}
			require.NoError(t, injector.Bind(ctx, input, WithPlan(plan), WithSource(input), WithResolutionGroupConcurrency(limit), WithResolutionGroupController(dependencyController{})))
			require.Equal(t, 8, input.First)
			require.Equal(t, 8, input.Second)
			require.LessOrEqual(t, peak.Load(), int32(limit))
			require.Zero(t, active.Load())
		})
	}
}

func TestDependencyGroupRequiresExplicitNestedSuspension(t *testing.T) {
	var childOpens atomic.Int32
	childInjector, err := NewInjector(WithProviders(groupProvider{"component", func(context.Context, string) (any, bool, error) { return 1, true, nil }}))
	require.NoError(t, err)
	childPlan, err := childInjector.CompilePlan(reflect.TypeFor[dependencyInput](), dependencySpecs("First")...)
	require.NoError(t, err)
	injector, err := NewInjector(WithProviders(groupProvider{"component", func(ctx context.Context, _ string) (any, bool, error) {
		child := &dependencyInput{}
		err := childInjector.Bind(ctx, child, WithPlan(childPlan), WithSource(child), WithResolutionGroupController(dependencyController{open: func(ResolutionGroupPlan) (ResolutionGroupRun, error) {
			childOpens.Add(1)
			return &groupRun{}, nil
		}}))
		return nil, false, err
	}}))
	require.NoError(t, err)
	plan, err := injector.CompilePlan(reflect.TypeFor[dependencyInput](), dependencySpecs("First")...)
	require.NoError(t, err)
	input := &dependencyInput{}
	err = injector.Bind(dependencyContext(t), input, WithPlan(plan), WithSource(input), WithResolutionGroupConcurrency(1), WithResolutionGroupController(dependencyController{}))
	require.ErrorContains(t, err, "SuspendResolutionGroup")
	require.Zero(t, childOpens.Load())
}

func TestDependencyGroupNestedFieldScratchIsDetached(t *testing.T) {
	type nested struct{ First, Second int }
	type input struct{ Nested *nested }
	injector, err := NewInjector(WithProviders(groupProvider{"component", func(context.Context, string) (any, bool, error) { return 7, true, nil }}))
	require.NoError(t, err)
	specs := dependencySpecs("Nested.First", "Nested.Second")
	plan, err := injector.CompilePlan(reflect.TypeFor[input](), specs...)
	require.NoError(t, err)
	for range 20 {
		original := &nested{First: 1, Second: 2}
		target := &input{Nested: original}
		err := injector.Bind(dependencyContext(t), target, WithPlan(plan), WithSource(target), WithResolutionGroupController(dependencyController{}), WithBindingObserver(func(_ context.Context, _ BindingEvent) error {
			// Publication and observation are serialized, including nested fields.
			if target.Nested.First != 1 && target.Nested.First != 7 || target.Nested.Second != 2 && target.Nested.Second != 7 {
				return errors.New("invalid nested publication")
			}
			return nil
		}))
		require.NoError(t, err)
		require.Equal(t, &nested{7, 7}, target.Nested)
	}
	specs = dependencySpecs("Nested", "Nested.First")
	_, err = injector.CompilePlan(reflect.TypeFor[input](), specs...)
	require.ErrorContains(t, err, "overlapping")
}

type dependencyFailingRun struct {
	groupRun
	observeError error
	closeError   error
}

func (r *dependencyFailingRun) Observe(ctx context.Context, outcome ResolutionOutcome) error {
	r.groupRun.Observe(ctx, outcome)
	return r.observeError
}

func (r *dependencyFailingRun) Close(ctx context.Context) error {
	r.groupRun.Close(ctx)
	return r.closeError
}

func TestDependencyGroupControllerFailuresRemainVisible(t *testing.T) {
	for _, failure := range []string{"enter", "observe", "close"} {
		t.Run(failure, func(t *testing.T) {
			cause := errors.New(failure)
			var calls atomic.Int32
			injector, err := NewInjector(WithProviders(groupProvider{"component", func(context.Context, string) (any, bool, error) {
				calls.Add(1)
				return 7, true, nil
			}}))
			require.NoError(t, err)
			plan, err := injector.CompilePlan(reflect.TypeFor[dependencyInput](), dependencySpecs("First", "Second")...)
			require.NoError(t, err)
			run := &dependencyFailingRun{}
			switch failure {
			case "enter":
				run.denied = map[string]error{"First": cause, "Second": cause}
			case "observe":
				run.observeError = cause
			case "close":
				run.closeError = cause
			}
			input := &dependencyInput{}
			err = injector.Bind(dependencyContext(t), input, WithPlan(plan), WithSource(input), WithResolutionGroupController(dependencyController{open: func(ResolutionGroupPlan) (ResolutionGroupRun, error) { return run, nil }}))
			require.ErrorIs(t, err, cause)
			require.True(t, run.closed)
			require.Len(t, run.outcomes, 2)
			if failure == "enter" {
				require.Zero(t, calls.Load())
			}
		})
	}
}

type dependencySnapshotProvider struct {
	resolve func(context.Context, *structology.State, string) (any, bool, error)
}

func (dependencySnapshotProvider) Kind() string  { return "component" }
func (dependencySnapshotProvider) Priority() int { return 0 }
func (p dependencySnapshotProvider) Locate(source *structology.State) locator.Locator {
	return groupLocator{p: groupProvider{"component", func(ctx context.Context, name string) (any, bool, error) {
		return p.resolve(ctx, source, name)
	}}}
}

func TestDependencyGroupPublicationDoesNotMutatePeerSourceSnapshot(t *testing.T) {
	ctx := dependencyContext(t)
	type nested struct{ First, Second int }
	type input struct{ Nested, Alias *nested }
	firstSnapshot := make(chan struct{})
	secondPublished := make(chan struct{})
	injector, err := NewInjector(WithProviders(dependencySnapshotProvider{resolve: func(ctx context.Context, source *structology.State, name string) (any, bool, error) {
		if name == "Nested.Second" {
			select {
			case <-firstSnapshot:
				return 7, true, nil
			case <-ctx.Done():
				return nil, false, ctx.Err()
			}
		}
		snapshot := source.State().(*input)
		close(firstSnapshot)
		select {
		case <-secondPublished:
		case <-ctx.Done():
			return nil, false, ctx.Err()
		}
		if snapshot.Nested.Second != 2 || snapshot.Alias.Second != 2 {
			return nil, false, errors.New("peer publication mutated retained source snapshot")
		}
		return 7, true, nil
	}}))
	require.NoError(t, err)
	plan, err := injector.CompilePlan(reflect.TypeFor[input](), dependencySpecs("Nested.First", "Nested.Second")...)
	require.NoError(t, err)
	original := &nested{1, 2}
	target := &input{Nested: original, Alias: original}
	err = injector.Bind(ctx, target, WithPlan(plan), WithSource(target), WithResolutionGroupController(dependencyController{}), WithBindingObserver(func(_ context.Context, event BindingEvent) error {
		if event.Path == "Nested.Second" {
			close(secondPublished)
		}
		return nil
	}))
	require.NoError(t, err)
	require.Equal(t, &nested{7, 7}, target.Nested)
	require.Equal(t, &nested{1, 2}, original)
}
