package bindly

import (
	"context"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/viant/bindly/locator"
	"github.com/viant/bindly/state"
	"github.com/viant/structology"
)

type countingProvider struct {
	kind      string
	cacheable bool
	delay     time.Duration
	mu        sync.Mutex
	calls     int
}

func (p *countingProvider) Kind() string           { return p.kind }
func (p *countingProvider) Priority() int          { return 1 }
func (p *countingProvider) DefaultCacheable() bool { return p.cacheable }
func (p *countingProvider) Locate(*structology.State) locator.Locator {
	return &countingLocator{provider: p}
}

type countingLocator struct{ provider *countingProvider }

func (l *countingLocator) Kind() string { return l.provider.kind }
func (l *countingLocator) Value(context.Context, reflect.Type, string) (interface{}, bool, error) {
	if l.provider.delay > 0 {
		time.Sleep(l.provider.delay)
	}
	l.provider.mu.Lock()
	defer l.provider.mu.Unlock()
	l.provider.calls++
	return l.provider.calls, true, nil
}

func (p *countingProvider) callCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

func TestValueCacheFollowsBindingAndProviderPolicy(t *testing.T) {
	type target struct{ Value int }
	tests := []struct {
		name             string
		providerDefault  bool
		bindingCacheable *bool
		wantCalls        int
		wantSecond       int
	}{
		{name: "provider default cacheable", providerDefault: true, wantCalls: 1, wantSecond: 1},
		{name: "explicit cache disabled", providerDefault: true, bindingCacheable: boolPointer(false), wantCalls: 2, wantSecond: 2},
		{name: "explicit cache enabled", providerDefault: false, bindingCacheable: boolPointer(true), wantCalls: 1, wantSecond: 1},
		{name: "provider default non-cacheable", providerDefault: false, wantCalls: 2, wantSecond: 2},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			provider := &countingProvider{kind: "counter", cacheable: testCase.providerDefault}
			root, err := NewInjector(WithProviders(provider))
			if err != nil {
				t.Fatalf("NewInjector() error = %v", err)
			}
			plan, err := root.CompilePlan(reflect.TypeOf(target{}), BindingSpec{
				Path:      "Value",
				Name:      "logicalValue",
				Location:  state.Location{Kind: provider.kind, In: "value"},
				Cacheable: testCase.bindingCacheable,
			})
			if err != nil {
				t.Fatalf("CompilePlan() error = %v", err)
			}
			first := &target{}
			second := &target{}
			if err := root.Bind(context.Background(), first, WithPlan(plan)); err != nil {
				t.Fatalf("first Bind() error = %v", err)
			}
			if err := root.Bind(context.Background(), second, WithPlan(plan)); err != nil {
				t.Fatalf("second Bind() error = %v", err)
			}
			if calls := provider.callCount(); calls != testCase.wantCalls || second.Value != testCase.wantSecond {
				t.Fatalf("calls=%d second=%d, want calls=%d second=%d", calls, second.Value, testCase.wantCalls, testCase.wantSecond)
			}
		})
	}
}

func TestCacheableBindingResolvesOnceConcurrently(t *testing.T) {
	type target struct{ Value int }
	provider := &countingProvider{kind: "counter", cacheable: true, delay: 10 * time.Millisecond}
	injector, err := NewInjector(WithProviders(provider))
	if err != nil {
		t.Fatalf("NewInjector() error = %v", err)
	}
	plan, err := injector.CompilePlan(reflect.TypeOf(target{}), BindingSpec{
		Path: "Value", Name: "logicalValue", Location: state.Location{Kind: provider.kind, In: "value"},
	})
	if err != nil {
		t.Fatalf("CompilePlan() error = %v", err)
	}

	const workers = 8
	results := make(chan int, workers)
	errors := make(chan error, workers)
	var wait sync.WaitGroup
	for index := 0; index < workers; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			actual := &target{}
			if err := injector.Bind(context.Background(), actual, WithPlan(plan)); err != nil {
				errors <- err
				return
			}
			results <- actual.Value
		}()
	}
	wait.Wait()
	close(errors)
	close(results)
	for err := range errors {
		t.Fatalf("Bind() error = %v", err)
	}
	for result := range results {
		if result != 1 {
			t.Fatalf("Value = %d, want 1", result)
		}
	}
	if calls := provider.callCount(); calls != 1 {
		t.Fatalf("provider calls = %d, want 1", calls)
	}
}

func TestForScopeOwnsInvocationValueCache(t *testing.T) {
	type target struct{ Value int }
	provider := &countingProvider{kind: "counter", cacheable: true}
	root, err := NewInjector(WithProviders(provider))
	if err != nil {
		t.Fatalf("NewInjector() error = %v", err)
	}
	plan, err := root.CompilePlan(reflect.TypeOf(target{}), BindingSpec{
		Path: "Value", Name: "logicalValue", Location: state.Location{Kind: provider.kind, In: "value"},
	})
	if err != nil {
		t.Fatalf("CompilePlan() error = %v", err)
	}
	firstScope, err := root.ForScope()
	if err != nil {
		t.Fatalf("first ForScope() error = %v", err)
	}
	secondScope, err := root.ForScope()
	if err != nil {
		t.Fatalf("second ForScope() error = %v", err)
	}
	first := &target{}
	second := &target{}
	if err := firstScope.Bind(context.Background(), first, WithPlan(plan)); err != nil {
		t.Fatalf("first Bind() error = %v", err)
	}
	if err := secondScope.Bind(context.Background(), second, WithPlan(plan)); err != nil {
		t.Fatalf("second Bind() error = %v", err)
	}
	if first.Value != 1 || second.Value != 2 {
		t.Fatalf("scope caches leaked: first=%d second=%d", first.Value, second.Value)
	}
}
