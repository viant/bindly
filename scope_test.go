package bindly_test

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/viant/bindly"
	"github.com/viant/bindly/locator"
	"github.com/viant/bindly/provider/values"
	"github.com/viant/bindly/state"
	"github.com/viant/structology"
)

type counterProvider struct{ count int }

func (p *counterProvider) Kind() string                              { return "count" }
func (p *counterProvider) Priority() int                             { return 0 }
func (p *counterProvider) DefaultCacheable() bool                    { return true }
func (p *counterProvider) Locate(*structology.State) locator.Locator { return p }
func (p *counterProvider) Value(context.Context, reflect.Type, string) (any, bool, error) {
	p.count++
	return p.count, true, nil
}

func TestBindingPolicies(t *testing.T) {
	ctx := context.Background()
	for _, cacheable := range []bool{true, false} {
		t.Run(map[bool]string{true: "cached", false: "uncached"}[cacheable], func(t *testing.T) {
			provider := &counterProvider{}
			injector, err := bindly.NewInjector(bindly.WithProviders(provider))
			if err != nil {
				t.Fatal(err)
			}
			type target struct{ A, B int }
			plan, err := injector.CompilePlan(reflect.TypeOf(target{}), bindly.BindingSpec{Path: "A", Location: state.Location{Kind: "count"}, Cacheable: &cacheable}, bindly.BindingSpec{Path: "B", Location: state.Location{Kind: "count"}, Cacheable: &cacheable})
			if err != nil {
				t.Fatal(err)
			}
			var actual target
			if err = injector.Bind(ctx, &actual, bindly.WithPlan(plan)); err != nil {
				t.Fatal(err)
			}
			want := 2
			if cacheable {
				want = 1
			}
			if provider.count != want || actual.B != want {
				t.Fatalf("count %d target %+v", provider.count, actual)
			}
		})
	}
	t.Run("required safe error", func(t *testing.T) {
		required := true
		injector, _ := bindly.NewInjector()
		target := struct{ ID int }{}
		plan, err := injector.CompilePlan(reflect.TypeOf(target), bindly.BindingSpec{Path: "ID", Location: state.Location{Kind: "query", In: "id"}, Required: &required, ErrorCode: 422, ErrorMessage: "ID is required"})
		if err != nil {
			t.Fatal(err)
		}
		err = injector.Bind(ctx, &target, bindly.WithPlan(plan))
		var bindingError *bindly.BindingError
		if !errors.As(err, &bindingError) || bindingError.StatusCode() != 422 || err.Error() != "ID is required" {
			t.Fatalf("error %v", err)
		}
	})
	t.Run("nil masks parent", func(t *testing.T) {
		root, _ := bindly.NewInjector(bindly.WithProviders(values.New("query", map[string]any{"id": 7})))
		scope, _ := root.ForScope(values.New("query", map[string]any{"id": nil}))
		target := struct {
			ID *int `bind:"kind=query,in=id"`
		}{}
		if err := scope.Bind(ctx, &target); err != nil {
			t.Fatal(err)
		}
		if target.ID != nil {
			t.Fatal("nil did not mask parent")
		}
	})
}

func TestBindingContextExplicitCache(t *testing.T) {
	provider := &counterProvider{}
	injector, err := bindly.NewInjector(bindly.WithProviders(provider))
	if err != nil {
		t.Fatal(err)
	}
	type target struct {
		A int `bind:"kind=count,cacheable"`
	}
	cache := bindly.NewValueCache()
	for i := 0; i < 2; i++ {
		var actual target
		scope := bindly.WithState[target](injector, struct{}{}, bindly.WithCache[target](cache))
		if err = scope.Inject(context.Background(), &actual); err != nil {
			t.Fatal(err)
		}
		if actual.A != 1 {
			t.Fatalf("cached value %d", actual.A)
		}
	}
	if provider.count != 1 {
		t.Fatalf("provider called %d times", provider.count)
	}
}

func TestPersistentCacheRoundTrip(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "values.gob")
	saved := bindly.NewValueCache()
	saved.Put("key", 7)
	if err := saved.Save(ctx, path); err != nil {
		t.Fatal(err)
	}
	loaded := bindly.NewValueCache()
	if err := loaded.Load(ctx, path); err != nil {
		t.Fatal(err)
	}
	if value, found := loaded.Get("key"); !found || value != 7 {
		t.Fatalf("loaded %v %v", value, found)
	}
	if err := loaded.Load(ctx, filepath.Join(t.TempDir(), "missing.gob")); err != nil {
		t.Fatal(err)
	}
}

func TestPersistentCacheDoesNotMaskChild(t *testing.T) {
	ctx := context.Background()
	cache := bindly.NewValueCache()
	root, _ := bindly.NewInjector(bindly.WithProviders(values.New("query", map[string]any{"id": 1})))
	type input struct {
		ID int `bind:"kind=query,in=id,cacheable"`
	}
	var parent input
	if err := bindly.WithState[input](root, struct{}{}, bindly.WithCache[input](cache)).Inject(ctx, &parent); err != nil {
		t.Fatal(err)
	}
	child, _ := root.ForScope(values.New("query", map[string]any{"id": 2}))
	var actual input
	if err := bindly.WithState[input](child, struct{}{}, bindly.WithCache[input](cache)).Inject(ctx, &actual); err != nil {
		t.Fatal(err)
	}
	if actual.ID != 2 {
		t.Fatalf("parent cache masked child: %+v", actual)
	}
}

func TestRequiredRejectsNull(t *testing.T) {
	var typedNil *int
	for _, value := range []any{nil, typedNil} {
		injector, _ := bindly.NewInjector(bindly.WithProviders(values.New("query", map[string]any{"id": value})))
		actual := struct {
			ID int `bind:"kind=query,in=id,required"`
		}{}
		if err := injector.Bind(context.Background(), &actual); err == nil {
			t.Fatalf("required null %T accepted", value)
		}
	}
}

type nestedProvider struct{}

func (*nestedProvider) Kind() string                                { return "nested" }
func (*nestedProvider) Priority() int                               { return locator.PriorityDependent }
func (p *nestedProvider) Locate(*structology.State) locator.Locator { return p }
func (*nestedProvider) Value(context.Context, reflect.Type, string) (any, bool, error) {
	return nil, false, errors.New("scope required")
}
func (*nestedProvider) ValueInScope(ctx context.Context, scope locator.Scope, _ reflect.Type, _ string) (any, bool, error) {
	var target struct {
		Value int `bind:"kind=query,in=id"`
	}
	if err := scope.BindTarget(ctx, &target); err != nil {
		return nil, false, err
	}
	return target.Value, true, nil
}
func TestScopedBindTarget(t *testing.T) {
	injector, err := bindly.NewInjector(bindly.WithProviders(&nestedProvider{}, values.New("query", map[string]any{"id": "7"})))
	if err != nil {
		t.Fatal(err)
	}
	var target struct {
		Value int `bind:"kind=nested"`
	}
	if err = injector.Bind(context.Background(), &target); err != nil {
		t.Fatal(err)
	}
	if target.Value != 7 {
		t.Fatalf("nested value %d", target.Value)
	}
}
