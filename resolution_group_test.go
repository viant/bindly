package bindly

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/viant/bindly/locator"
	"github.com/viant/bindly/state"
	"github.com/viant/structology"
)

type groupProvider struct {
	kind    string
	resolve func(context.Context, string) (any, bool, error)
}

func (p groupProvider) Kind() string                              { return p.kind }
func (p groupProvider) Priority() int                             { return 0 }
func (p groupProvider) Locate(*structology.State) locator.Locator { return groupLocator{p: p} }

type groupLocator struct{ p groupProvider }

func (l groupLocator) Kind() string { return l.p.kind }
func (l groupLocator) Value(ctx context.Context, _ reflect.Type, name string) (any, bool, error) {
	return l.p.resolve(ctx, name)
}

type groupController struct {
	run     *groupRun
	opens   int
	barrier func()
}

func (c *groupController) Open(_ context.Context, p ResolutionGroupPlan) (ResolutionGroupRun, error) {
	c.opens++
	if c.barrier != nil {
		c.barrier()
	}
	members := p.Members()
	members[0].ResolutionGroup.Name = "mutated"
	if p.Members()[0].ResolutionGroup.Name == "mutated" {
		panic("mutable descriptor")
	}
	return c.run, nil
}

type groupRun struct {
	mu       sync.Mutex
	outcomes []ResolutionOutcome
	closed   bool
	observed chan string
	denied   map[string]error
}

func (r *groupRun) Enter(ctx context.Context, path string) (context.Context, error) {
	if err := r.denied[path]; err != nil {
		return nil, err
	}
	return ctx, nil
}
func (r *groupRun) Observe(_ context.Context, o ResolutionOutcome) error {
	r.mu.Lock()
	r.outcomes = append(r.outcomes, o)
	r.mu.Unlock()
	if r.observed != nil {
		r.observed <- o.Path()
	}
	return nil
}
func (r *groupRun) Close(context.Context) error { r.closed = true; return nil }

type groupInput struct {
	Jwt              string
	Data             *struct{ ID int }
	AdvertiserId     int
	Auth             *struct{ ID int }
	CurAdvertiser    int
	CurTaxonomies    int
	CurUamTaxonomies int
	Has              *groupHas `setMarker:"true"`
}
type groupHas struct{ Jwt, Data, AdvertiserId, Auth, CurAdvertiser, CurTaxonomies, CurUamTaxonomies bool }

func groupSpecs() []BindingSpec {
	required := true
	result := []BindingSpec{{Path: "Jwt", Location: state.Location{Kind: "header", In: "jwt"}, Required: &required}, {Path: "Data", Location: state.Location{Kind: "body"}, Required: &required}}
	for _, name := range []string{"AdvertiserId", "Auth", "CurAdvertiser", "CurTaxonomies", "CurUamTaxonomies"} {
		kind := "component"
		if name == "AdvertiserId" {
			kind = "path"
		}
		g := &ResolutionGroupSpec{Name: "archive_current", After: []string{"Jwt", "Data"}}
		if name != "AdvertiserId" && name != "Auth" {
			g.DependsOn = []string{"AdvertiserId"}
		}
		result = append(result, BindingSpec{Path: name, Name: name, Location: state.Location{Kind: kind, In: name}, ResolutionGroup: g, Required: &required})
	}
	return result
}
func TestResolutionGroupConcurrentCompletionAndBarriers(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	entered := make(chan string, 5)
	releases := map[string]chan struct{}{}
	causes := map[string]error{}
	for _, name := range []string{"AdvertiserId", "Auth", "CurAdvertiser", "CurTaxonomies", "CurUamTaxonomies"} {
		releases[name] = make(chan struct{})
		causes[name] = errors.New(name + " actual cause")
	}
	resolve := func(ctx context.Context, name string) (any, bool, error) {
		entered <- name
		select {
		case <-releases[name]:
			return nil, false, causes[name]
		case <-ctx.Done():
			return nil, false, ctx.Err()
		}
	}
	data := &struct{ ID int }{7}
	injector, err := NewInjector(WithProviders(groupProvider{"path", resolve}, groupProvider{"component", resolve}, groupProvider{"header", func(context.Context, string) (any, bool, error) { return "jwt", true, nil }}, groupProvider{"body", func(context.Context, string) (any, bool, error) { return data, true, nil }}))
	if err != nil {
		t.Fatal(err)
	}
	plan, err := injector.CompilePlan(reflect.TypeOf(groupInput{}), groupSpecs()...)
	if err != nil {
		t.Fatal(err)
	}
	input := &groupInput{}
	run := &groupRun{observed: make(chan string, 5)}
	controller := &groupController{run: run, barrier: func() {
		if input.Jwt != "jwt" || input.Data != data || !input.Has.Jwt || !input.Has.Data {
			t.Fatal("barriers or pointer identity lost")
		}
	}}
	done := make(chan error, 1)
	go func() {
		done <- injector.Bind(ctx, input, WithPlan(plan), WithSource(input), WithResolutionGroupController(controller))
	}()
	for n := 0; n < 5; n++ {
		select {
		case <-entered:
		case <-ctx.Done():
			t.Fatal("members not launched concurrently")
		}
	}
	order := []string{"CurTaxonomies", "CurAdvertiser", "AdvertiserId", "CurUamTaxonomies", "Auth"}
	for _, name := range order {
		close(releases[name])
		select {
		case actual := <-run.observed:
			if actual != name {
				t.Fatalf("completion got %s want %s", actual, name)
			}
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	err = <-done
	var grouped *ResolutionGroupError
	if !errors.As(err, &grouped) {
		t.Fatalf("missing grouped failures: %v", err)
	}
	failures := grouped.Failures()
	if len(failures) != 5 {
		t.Fatalf("failures=%d", len(failures))
	}
	for index, failure := range failures {
		if failure.Path != order[index] || !errors.Is(failure, causes[order[index]]) {
			t.Fatalf("wrong actual failure %v", failure)
		}
	}
	failures[0].Path = "mutated"
	if grouped.Failures()[0].Path == "mutated" {
		t.Fatal("mutable public errors")
	}
	if !run.closed || controller.opens != 1 {
		t.Fatal("group not closed")
	}
	if input.Has.AdvertiserId || input.Has.Auth || input.Has.CurAdvertiser {
		t.Fatal("failed field marked present")
	}
}

func TestResolutionGroupRejectsBeforeWorkAndDefaultsUnchanged(t *testing.T) {
	calls := 0
	injector, _ := NewInjector(WithProviders(groupProvider{"path", func(context.Context, string) (any, bool, error) {
		calls++
		return nil, false, errors.New("path failure")
	}}))
	specs := groupSpecs()
	plan, err := injector.CompilePlan(reflect.TypeOf(groupInput{}), specs...)
	if err != nil {
		t.Fatal(err)
	}
	if err := injector.Bind(context.Background(), &groupInput{}, WithPlan(plan)); err == nil || calls != 0 {
		t.Fatal("missing controller did not reject before work")
	}
	specs[2].ResolutionGroup.DependsOn = []string{"CurAdvertiser"}
	if _, err := injector.CompilePlan(reflect.TypeOf(groupInput{}), specs...); err == nil {
		t.Fatal("cycle accepted")
	}
	plain := []BindingSpec{{Path: "AdvertiserId", Location: state.Location{Kind: "path", In: "one"}}, {Path: "CurAdvertiser", Location: state.Location{Kind: "path", In: "two"}, Async: true}}
	plan, err = injector.CompilePlan(reflect.TypeOf(groupInput{}), plain...)
	if err != nil {
		t.Fatal(err)
	}
	err = injector.Bind(context.Background(), &groupInput{}, WithPlan(plan))
	if err == nil || calls != 1 {
		t.Fatal("metadata absent fail-fast changed")
	}
}

func TestResolutionGroupPublishesActualSuccessAndObserverTargetBeforeJoin(t *testing.T) {
	entered := make(chan string, 5)
	release := make(chan struct{})
	authObserved := make(chan struct{})
	auth := &struct{ ID int }{19}
	data := &struct{ ID int }{7}
	actualFailure := errors.New("actual advertiser conversion failure")
	resolve := func(ctx context.Context, name string) (any, bool, error) {
		entered <- name
		if name == "Auth" {
			return auth, true, nil
		}
		select {
		case <-release:
		case <-ctx.Done():
			return nil, false, ctx.Err()
		}
		if name == "AdvertiserId" {
			return nil, false, actualFailure
		}
		return 31, true, nil
	}
	injector, err := NewInjector(WithProviders(groupProvider{"path", resolve}, groupProvider{"component", resolve}, groupProvider{"header", func(context.Context, string) (any, bool, error) { return "jwt", true, nil }}, groupProvider{"body", func(context.Context, string) (any, bool, error) { return data, true, nil }}))
	if err != nil {
		t.Fatal(err)
	}
	plan, err := injector.CompilePlan(reflect.TypeFor[groupInput](), groupSpecs()...)
	if err != nil {
		t.Fatal(err)
	}
	input := &groupInput{}
	run := &groupRun{}
	controller := &groupController{run: run}
	done := make(chan error, 1)
	go func() {
		done <- injector.Bind(t.Context(), input, WithPlan(plan), WithSource(input), WithResolutionGroupController(controller), WithBindingObserver(func(_ context.Context, event BindingEvent) error {
			if event.Path == "Auth" {
				if event.Target != input || event.Value != auth || input.Auth != auth || !input.Has.Auth || input.Data != data {
					t.Error("partial success lost original pointer/target/marker")
				}
				close(authObserved)
			}
			return nil
		}))
	}()
	for n := 0; n < 5; n++ {
		<-entered
	}
	<-authObserved
	// The observer ran while all other real bindings remain blocked; no atomic
	// all-member publication guarantee is invented beyond the source contract.
	close(release)
	err = <-done
	var group *ResolutionGroupError
	if !errors.As(err, &group) || len(group.Failures()) != 1 || group.Failures()[0].Path != "AdvertiserId" || !errors.Is(err, actualFailure) {
		t.Fatalf("successful peer fabricated as failure: %v", err)
	}
	if input.Auth != auth || input.Data != data || !input.Has.Auth || input.Has.AdvertiserId || !input.Has.CurTaxonomies {
		t.Fatal("success aliases/presence or failed presence changed")
	}
}

func TestResolutionGroupDeniedAdmissionDoesNotFabricateFailedView(t *testing.T) {
	denied := errors.New("actual denied admission")
	injector, err := NewInjector(WithProviders(groupProvider{"path", func(context.Context, string) (any, bool, error) { return nil, false, errors.New("actual path failure") }}, groupProvider{"component", func(context.Context, string) (any, bool, error) { return nil, false, errors.New("actual read failure") }}, groupProvider{"header", func(context.Context, string) (any, bool, error) { return "jwt", true, nil }}, groupProvider{"body", func(context.Context, string) (any, bool, error) { return &struct{ ID int }{7}, true, nil }}))
	if err != nil {
		t.Fatal(err)
	}
	plan, err := injector.CompilePlan(reflect.TypeFor[groupInput](), groupSpecs()...)
	if err != nil {
		t.Fatal(err)
	}
	run := &groupRun{denied: map[string]error{"Auth": denied}}
	input := &groupInput{}
	err = injector.Bind(t.Context(), input, WithPlan(plan), WithSource(input), WithResolutionGroupController(&groupController{run: run}))
	var group *ResolutionGroupError
	if !errors.As(err, &group) || len(group.Failures()) != 4 {
		t.Fatalf("actual attempts missing %v", err)
	}
	for _, failure := range group.Failures() {
		if failure.Path == "Auth" {
			t.Fatal("denied admission fabricated failed view")
		}
	}
	for _, outcome := range run.outcomes {
		if outcome.Path() == "Auth" {
			if outcome.Attempted() || outcome.Failure() != nil || !errors.Is(outcome.Cause(), denied) {
				t.Fatal("denied admission provenance lost")
			}
			return
		}
	}
	t.Fatal("actual denied outcome omitted")
}

func TestResolutionGroupBindingPanicRetainsActualErrorAndTerminalProvenance(t *testing.T) {
	actual := errors.New("actual provider panic")
	injector, err := NewInjector(WithProviders(groupProvider{"path", func(context.Context, string) (any, bool, error) { return 31, true, nil }}, groupProvider{"component", func(context.Context, string) (any, bool, error) { panic(actual) }}, groupProvider{"header", func(context.Context, string) (any, bool, error) { return "jwt", true, nil }}, groupProvider{"body", func(context.Context, string) (any, bool, error) { return &struct{ ID int }{7}, true, nil }}))
	if err != nil {
		t.Fatal(err)
	}
	plan, err := injector.CompilePlan(reflect.TypeFor[groupInput](), groupSpecs()...)
	if err != nil {
		t.Fatal(err)
	}
	run := &groupRun{}
	input := &groupInput{}
	err = injector.Bind(t.Context(), input, WithPlan(plan), WithSource(input), WithResolutionGroupController(&groupController{run: run}))
	if !errors.Is(err, actual) {
		t.Fatalf("actual panic error lost %v", err)
	}
	found := false
	for _, outcome := range run.outcomes {
		if outcome.Terminal() && errors.Is(outcome.Cause(), actual) {
			found = true
		}
	}
	if !found || !run.closed {
		t.Fatal("panic terminal provenance or joined closure missing")
	}
}
