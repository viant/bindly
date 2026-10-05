package bindly

import (
	"context"
	"reflect"
	"testing"

	"github.com/viant/bindly/state"
)

func TestCompilePlanCopiesBooleanMetadata(t *testing.T) {
	type target struct{ Value int }
	cacheable := true
	provider := &countingProvider{kind: "counter", cacheable: false}
	injector, err := NewInjector(WithProviders(provider))
	if err != nil {
		t.Fatalf("NewInjector() error = %v", err)
	}
	plan, err := injector.CompilePlan(reflect.TypeOf(target{}), BindingSpec{
		Path: "Value", Location: state.Location{Kind: provider.kind}, Cacheable: &cacheable,
	})
	if err != nil {
		t.Fatalf("CompilePlan() error = %v", err)
	}
	cacheable = false

	first := &target{}
	second := &target{}
	if err := injector.Bind(context.Background(), first, WithPlan(plan)); err != nil {
		t.Fatalf("first Bind() error = %v", err)
	}
	if err := injector.Bind(context.Background(), second, WithPlan(plan)); err != nil {
		t.Fatalf("second Bind() error = %v", err)
	}
	if calls := provider.callCount(); calls != 1 {
		t.Fatalf("provider calls = %d, want plan-owned cacheable metadata", calls)
	}
}
