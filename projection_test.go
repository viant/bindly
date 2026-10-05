package bindly

import (
	"reflect"
	"strings"
	"testing"

	"github.com/viant/bindly/state"
)

func TestPlanProjectionUsesCompiledBindingsAndExplicitFields(t *testing.T) {
	type nested struct {
		Region string
	}
	type target struct {
		AccountID string
		Nested    *nested
		Computed  int
	}
	injector, err := NewInjector()
	if err != nil {
		t.Fatalf("NewInjector() error = %v", err)
	}
	plan, err := injector.CompilePlan(reflect.TypeOf(target{}),
		BindingSpec{Path: "AccountID", Name: "Account", Location: state.Location{Kind: "query", In: "account_id"}},
		BindingSpec{Path: "Nested.Region", Name: "Region", Location: state.Location{Kind: "query", In: "region_code"}},
	)
	if err != nil {
		t.Fatalf("CompilePlan() error = %v", err)
	}
	projection, err := plan.Projection(ProjectionField{Path: "Computed", Names: []string{"score"}})
	if err != nil {
		t.Fatalf("Projection() error = %v", err)
	}
	if projection.TargetType() != reflect.TypeOf(target{}) {
		t.Fatalf("TargetType() = %v", projection.TargetType())
	}
	actual := &target{AccountID: "acct-7", Nested: &nested{Region: "EU"}, Computed: 9}
	for name, want := range map[string]any{
		"ACCOUNT": "acct-7", "account_id": "acct-7", "Nested.Region": "EU", "region_code": "EU", "score": 9,
	} {
		value, ok, valueErr := projection.Value(actual, name)
		if valueErr != nil || !ok || value != want {
			t.Fatalf("Value(%q) = %#v, %v, %v; want %#v", name, value, ok, valueErr, want)
		}
	}
}

func TestPlanProjectionHandlesNilNestedPointer(t *testing.T) {
	type nested struct{ Region string }
	type target struct{ Nested *nested }
	injector, _ := NewInjector()
	plan, err := injector.CompilePlan(reflect.TypeOf(target{}), BindingSpec{
		Path: "Nested.Region", Location: state.Location{Kind: "query", In: "region"},
	})
	if err != nil {
		t.Fatalf("CompilePlan() error = %v", err)
	}
	projection, err := plan.Projection()
	if err != nil {
		t.Fatalf("Projection() error = %v", err)
	}
	value, ok, err := projection.Value(&target{}, "region")
	if err != nil || !ok || value != nil {
		t.Fatalf("Value() = %#v, %v, %v; want nil, true, nil", value, ok, err)
	}
}

func TestPlanProjectionRejectsAliasCollisionAndWrongTarget(t *testing.T) {
	type target struct {
		First  int
		Second int
	}
	injector, _ := NewInjector()
	plan, err := injector.CompilePlan(reflect.TypeOf(target{}))
	if err != nil {
		t.Fatalf("CompilePlan() error = %v", err)
	}
	_, err = plan.Projection(
		ProjectionField{Path: "First", Names: []string{"id"}},
		ProjectionField{Path: "Second", Names: []string{"ID"}},
	)
	if err == nil || !strings.Contains(err.Error(), "targets both") {
		t.Fatalf("Projection() error = %v, want alias collision", err)
	}
	projection, err := plan.Projection(ProjectionField{Path: "First"})
	if err != nil {
		t.Fatalf("Projection() error = %v", err)
	}
	if _, _, err = projection.Value(struct{ First int }{}, "First"); err == nil {
		t.Fatal("expected target identity error")
	}
}

func TestPlanProjectionDoesNotExposeParamDependencySourceAsAlias(t *testing.T) {
	type target struct{ IDs []int }
	injector, _ := NewInjector()
	plan, err := injector.CompilePlan(reflect.TypeOf(target{}), BindingSpec{
		Path: "IDs", Name: "IDs", Location: state.Location{Kind: "param", In: "Events"},
	})
	if err != nil {
		t.Fatalf("CompilePlan() error = %v", err)
	}
	projection, err := plan.Projection()
	if err != nil {
		t.Fatalf("Projection() error = %v", err)
	}
	if _, ok, err := projection.Value(&target{IDs: []int{1}}, "Events"); err != nil || ok {
		t.Fatalf("Value(Events) = ok %v, err %v; want unknown alias", ok, err)
	}
}
