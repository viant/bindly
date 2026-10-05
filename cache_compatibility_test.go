package bindly

import (
	"context"
	"encoding/json"
	"github.com/viant/bindly/locator"
	"github.com/viant/bindly/locator/buildin"
	"github.com/viant/bindly/state"
	"github.com/viant/structology"
	"reflect"
	"testing"
)

func TestPersistentCacheSeparatesExplicitSources(t *testing.T) {
	type target struct{ Value int }
	injector, err := NewInjector(WithProviders(buildin.Struct("state", "", 1)))
	if err != nil {
		t.Fatal(err)
	}
	plan, err := injector.CompilePlan(reflect.TypeFor[target](), BindingSpec{Path: "Value", Location: state.Location{Kind: "state", In: "Value"}, Cacheable: boolPointer(true)})
	if err != nil {
		t.Fatal(err)
	}
	cache := NewValueCache()
	for _, explicit := range []bool{false, true} {
		for _, want := range []int{7, 9} {
			source := &target{Value: want}
			actual := &target{}
			options := []BindOption{WithPlan(plan), WithSource(source)}
			if explicit {
				options = append(options, WithValueCache(cache))
			}
			if err := injector.Bind(context.Background(), actual, options...); err != nil {
				t.Fatal(err)
			}
			if actual.Value != want {
				t.Fatalf("explicit cache %v: value=%d want=%d", explicit, actual.Value, want)
			}
		}
	}
}

func TestDefaultCacheDoesNotDiscardObservedMetadata(t *testing.T) {
	provider := &metadataProvider{value: locator.ValueWithMetadata{Value: 7, Metadata: "evidence"}, found: true}
	injector, err := NewInjector(WithProviders(provider))
	if err != nil {
		t.Fatal(err)
	}
	type target struct{ Value int }
	plan, err := injector.CompilePlan(reflect.TypeFor[target](), BindingSpec{Path: "Value", Location: state.Location{Kind: "metadata"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := injector.Bind(context.Background(), &target{}, WithPlan(plan)); err != nil {
		t.Fatal(err)
	}
	for pass := 0; pass < 2; pass++ {
		var event BindingEvent
		if err := injector.Bind(context.Background(), &target{}, WithPlan(plan), WithBindingObserver(func(_ context.Context, e BindingEvent) error { event = e; return nil })); err != nil {
			t.Fatal(err)
		}
		if event.Metadata != "evidence" {
			t.Fatalf("observed metadata=%v", event.Metadata)
		}
	}
	if provider.calls != 3 {
		t.Fatalf("provider calls=%d want=3", provider.calls)
	}
}

type cachedCaptureProvider struct{ valueCalls, captureCalls int }

func (p *cachedCaptureProvider) Kind() string                              { return "capture" }
func (p *cachedCaptureProvider) Priority() int                             { return 0 }
func (p *cachedCaptureProvider) DefaultCacheable() bool                    { return true }
func (p *cachedCaptureProvider) Locate(*structology.State) locator.Locator { return p }
func (p *cachedCaptureProvider) Value(context.Context, reflect.Type, string) (any, bool, error) {
	p.valueCalls++
	return 7, true, nil
}
func (p *cachedCaptureProvider) CaptureSource(context.Context, reflect.Type, string) (any, bool, error) {
	p.captureCalls++
	return json.RawMessage("8"), true, nil
}
func TestSourceCaptureBypassesDecodedPersistentValues(t *testing.T) {
	provider := &cachedCaptureProvider{}
	injector, err := NewInjector(WithProviders(provider))
	if err != nil {
		t.Fatal(err)
	}
	location := &state.Location{Kind: provider.Kind()}
	cache := NewValueCache()
	normal := &invocation{injector: injector, active: map[string]bool{}, persistent: cache}
	result, err := normal.resolveResult(context.Background(), location, reflect.TypeFor[int](), nil)
	if err != nil || result.value != 7 {
		t.Fatalf("normal=%+v err=%v", result, err)
	}
	capture := &invocation{injector: injector, active: map[string]bool{}, persistent: cache, captureSources: true}
	result, err = capture.resolveResult(context.Background(), location, reflect.TypeFor[int](), nil)
	if err != nil {
		t.Fatal(err)
	}
	raw, ok := result.value.(json.RawMessage)
	if !ok || string(raw) != "8" || provider.captureCalls != 1 || provider.valueCalls != 1 {
		t.Fatalf("capture=%+v calls=%+v", result, provider)
	}
}
