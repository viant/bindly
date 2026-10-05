package locator_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/viant/bindly"
	"github.com/viant/bindly/locator"
	"github.com/viant/bindly/state"
	"github.com/viant/structology"
)

type compositeTestProvider struct {
	kind      string
	priority  int
	cacheable bool
	locator   locator.Locator
}

func (p *compositeTestProvider) Kind() string                              { return p.kind }
func (p *compositeTestProvider) Priority() int                             { return p.priority }
func (p *compositeTestProvider) DefaultCacheable() bool                    { return p.cacheable }
func (p *compositeTestProvider) Locate(*structology.State) locator.Locator { return p.locator }

type compositeTestLocator struct {
	kind   string
	value  interface{}
	found  bool
	err    error
	scoped bool
}

func (l *compositeTestLocator) Kind() string { return l.kind }
func (l *compositeTestLocator) Value(context.Context, reflect.Type, string) (interface{}, bool, error) {
	return l.value, l.found, l.err
}
func (l *compositeTestLocator) ValueInScope(context.Context, locator.Scope, reflect.Type, string) (interface{}, bool, error) {
	l.scoped = true
	return l.value, l.found, l.err
}

func TestComposeProvidersResolvesByAuthorityThroughInjector(t *testing.T) {
	miss := &compositeTestProvider{kind: "query", priority: 100, cacheable: true, locator: &compositeTestLocator{kind: "query"}}
	foundNil := &compositeTestProvider{kind: "query", priority: 200, cacheable: true, locator: &compositeTestLocator{kind: "query", found: true}}
	lower := &compositeTestProvider{kind: "query", priority: 300, cacheable: true, locator: &compositeTestLocator{kind: "query", value: "lower", found: true}}
	provider, err := locator.ComposeProviders("query",
		locator.ProviderLayer{Name: "request", Provider: miss},
		locator.ProviderLayer{Name: "component", Provider: foundNil},
		locator.ProviderLayer{Name: "default", Provider: lower},
	)
	if err != nil {
		t.Fatal(err)
	}
	if provider.Priority() != 300 || !provider.(locator.CachePolicy).DefaultCacheable() {
		t.Fatalf("provider metadata = (%d, %v)", provider.Priority(), provider.(locator.CachePolicy).DefaultCacheable())
	}
	injector, err := bindly.NewInjector(bindly.WithProviders(provider))
	if err != nil {
		t.Fatal(err)
	}
	plan, err := injector.CompilePlan(reflect.TypeOf(struct{ Value interface{} }{}), bindly.BindingSpec{
		Path: "Value", Location: state.Location{Kind: "query", In: "value"},
	})
	if err != nil {
		t.Fatal(err)
	}
	target := &struct{ Value interface{} }{Value: "initial"}
	if err = injector.Bind(context.Background(), target, bindly.WithPlan(plan)); err != nil {
		t.Fatal(err)
	}
	if target.Value != nil {
		t.Fatalf("found nil must stop fallback, got %#v", target.Value)
	}
}

func TestComposeProvidersUsesScopedLocatorsAndStopsOnError(t *testing.T) {
	expected := errors.New("lookup failed")
	firstLocator := &compositeTestLocator{kind: "derived", err: expected}
	provider, err := locator.ComposeProviders("derived",
		locator.ProviderLayer{Name: "first", Provider: &compositeTestProvider{kind: "derived", locator: firstLocator}},
		locator.ProviderLayer{Name: "second", Provider: &compositeTestProvider{kind: "derived", locator: &compositeTestLocator{kind: "derived", value: 42, found: true}}},
	)
	if err != nil {
		t.Fatal(err)
	}
	actual := provider.Locate(nil).(locator.ScopedLocator)
	_, found, err := actual.ValueInScope(context.Background(), compositeTestScope{}, reflect.TypeOf(0), "value")
	if found || !errors.Is(err, expected) || !firstLocator.scoped {
		t.Fatalf("ValueInScope() = (found:%v err:%v scoped:%v)", found, err, firstLocator.scoped)
	}
}

func TestComposeProvidersValidatesLayersAndConservativeCache(t *testing.T) {
	cacheable := &compositeTestProvider{kind: "query", cacheable: true, locator: &compositeTestLocator{kind: "query"}}
	nonCacheable := &compositeTestProvider{kind: "query", cacheable: false, locator: &compositeTestLocator{kind: "query"}}
	provider, err := locator.ComposeProviders("query",
		locator.ProviderLayer{Name: "first", Provider: cacheable},
		locator.ProviderLayer{Name: "second", Provider: nonCacheable},
	)
	if err != nil || provider.(locator.CachePolicy).DefaultCacheable() {
		t.Fatalf("ComposeProviders() = (%v, %v)", provider, err)
	}
	for _, testCase := range []struct {
		name   string
		kind   string
		layers []locator.ProviderLayer
	}{
		{name: "empty kind"},
		{name: "empty layers", kind: "query"},
		{name: "duplicate layer", kind: "query", layers: []locator.ProviderLayer{{Name: "same", Provider: cacheable}, {Name: "same", Provider: cacheable}}},
		{name: "wrong kind", kind: "header", layers: []locator.ProviderLayer{{Name: "query", Provider: cacheable}}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if _, err := locator.ComposeProviders(testCase.kind, testCase.layers...); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

type compositeTestScope struct{}

func (compositeTestScope) Value(context.Context, *state.Location) (interface{}, bool, error) {
	return nil, false, nil
}
func (compositeTestScope) BindTarget(context.Context, interface{}) error { return nil }
