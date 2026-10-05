package bindly

import (
	"context"
	"reflect"
	"testing"

	"github.com/viant/bindly/locator"
	"github.com/viant/bindly/locator/buildin"
	"github.com/viant/bindly/state"
	"github.com/viant/structology"
)

type scopedProvider struct{}

func (p *scopedProvider) Kind() string  { return "derived" }
func (p *scopedProvider) Priority() int { return 1 }
func (p *scopedProvider) Locate(*structology.State) locator.Locator {
	return &scopedValueLocator{}
}

type scopedValueLocator struct{}

func (l *scopedValueLocator) Kind() string { return "derived" }
func (l *scopedValueLocator) Value(context.Context, reflect.Type, string) (interface{}, bool, error) {
	return nil, false, nil
}

type orderedDerivedProvider struct{}

func (p *orderedDerivedProvider) Kind() string  { return "orderedDerived" }
func (p *orderedDerivedProvider) Priority() int { return 200 }
func (p *orderedDerivedProvider) Locate(*structology.State) locator.Locator {
	return &orderedDerivedLocator{}
}

type orderedDerivedLocator struct{}

func (l *orderedDerivedLocator) Kind() string { return "orderedDerived" }
func (l *orderedDerivedLocator) Value(context.Context, reflect.Type, string) (interface{}, bool, error) {
	return nil, false, nil
}
func (l *orderedDerivedLocator) ValueInScope(ctx context.Context, scope locator.Scope, _ reflect.Type, _ string) (interface{}, bool, error) {
	return scope.Value(ctx, &state.Location{Kind: "boundInput", In: "Base"})
}
func (l *scopedValueLocator) ValueInScope(ctx context.Context, scope locator.Scope, _ reflect.Type, _ string) (interface{}, bool, error) {
	value, ok, err := scope.Value(ctx, &state.Location{Kind: "base", In: "Name"})
	if err != nil || !ok {
		return value, ok, err
	}
	target := &struct {
		Name string `bind:"kind=base,in=Name"`
	}{}
	if err := scope.BindTarget(ctx, target); err != nil {
		return nil, false, err
	}
	return value.(string) + ":" + target.Name, true, nil
}

func TestScopedLocatorUsesCurrentBindingScope(t *testing.T) {
	type source struct{ Name string }
	type target struct{ Value string }
	injector, err := NewInjector(WithProviders(
		buildin.Direct("base", source{Name: "Ada"}, 1),
		&scopedProvider{},
	))
	if err != nil {
		t.Fatalf("NewInjector() error = %v", err)
	}
	plan, err := injector.CompilePlan(reflect.TypeOf(target{}), BindingSpec{
		Path: "Value", Location: state.Location{Kind: "derived", In: "value"},
	})
	if err != nil {
		t.Fatalf("CompilePlan() error = %v", err)
	}
	actual := &target{}
	if err := injector.Bind(context.Background(), actual, WithPlan(plan), WithSource(&source{})); err != nil {
		t.Fatalf("Bind() error = %v", err)
	}
	if actual.Value != "Ada:Ada" {
		t.Fatalf("Value = %q, want Ada:Ada", actual.Value)
	}
}

func TestBindingOrderDefaultsToActiveProviderPriority(t *testing.T) {
	type target struct {
		Derived int
		Base    int
	}
	injector, err := NewInjector(WithProviders(
		buildin.Direct("base", struct{ Value int }{Value: 7}, 100),
		buildin.Struct("boundInput", "", 150),
		&orderedDerivedProvider{},
	))
	if err != nil {
		t.Fatalf("NewInjector() error = %v", err)
	}
	plan, err := injector.CompilePlan(reflect.TypeOf(target{}),
		BindingSpec{Path: "Derived", Location: state.Location{Kind: "orderedDerived"}},
		BindingSpec{Path: "Base", Location: state.Location{Kind: "base", In: "Value"}},
	)
	if err != nil {
		t.Fatalf("CompilePlan() error = %v", err)
	}
	actual := &target{}
	if err := injector.Bind(context.Background(), actual, WithPlan(plan), WithSource(actual)); err != nil {
		t.Fatalf("Bind() error = %v", err)
	}
	if actual.Base != 7 || actual.Derived != 7 {
		t.Fatalf("target = %+v, want Base=7 Derived=7", actual)
	}
}
