package bindly_test

import (
	"context"
	"net/http"
	"reflect"
	"testing"

	"github.com/viant/bindly"
	"github.com/viant/bindly/provider/values"
	"github.com/viant/bindly/state"
)

func TestPlanScopedBinding(t *testing.T) {
	type input struct {
		ID   int     `parameter:"identifier,kind=query,in=id,required=true"`
		Name *string `bind:"Name,kind=query,in=name"`
	}
	root, err := bindly.NewInjector(bindly.WithProviders(values.New("query", map[string]any{"name": "parent", "id": "1"})))
	if err != nil {
		t.Fatal(err)
	}
	plan, err := root.CompilePlan(reflect.TypeOf(input{}))
	if err != nil {
		t.Fatal(err)
	}
	projection, err := plan.Projection()
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name   string
		value  any
		want   int
		reject bool
	}{{"typed", "7", 7, false}, {"invalid", "wrong", 0, true}, {"zero", 0, 0, false}} {
		t.Run(tt.name, func(t *testing.T) {
			scope, err := root.ForScope(values.New("query", map[string]any{"id": tt.value}))
			if err != nil {
				t.Fatal(err)
			}
			var actual input
			err = scope.Bind(context.Background(), &actual, bindly.WithPlan(plan))
			if tt.reject {
				if err == nil {
					t.Fatal("expected conversion error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if actual.ID != tt.want || actual.Name == nil || *actual.Name != "parent" {
				t.Fatalf("input %+v", actual)
			}
			for _, name := range []string{"ID", "id", "identifier"} {
				value, found, err := projection.Value(&actual, name)
				if err != nil || !found || value != tt.want {
					t.Fatalf("projection %s=%v %v %v", name, value, found, err)
				}
			}
		})
	}
}

func TestPlanBindingTagAmbiguity(t *testing.T) {
	field := reflect.TypeOf(struct {
		ID int `bind:"kind=query,in=id" parameter:"kind=query,in=id"`
	}{}).Field(0)
	if _, _, err := bindly.BindingSpecFromField(field); err == nil {
		t.Fatal("ambiguous binding accepted")
	}
}

func TestBareRequiredTag(t *testing.T) {
	field := reflect.TypeOf(struct {
		ID int `parameter:"ID,kind=path,in=id,required"`
	}{}).Field(0)
	spec, found, err := bindly.BindingSpecFromField(field)
	if err != nil || !found || spec.Required == nil || !*spec.Required {
		t.Fatalf("spec %+v found %v err %v", spec, found, err)
	}
}

func TestBindingQuotedScalars(t *testing.T) {
	for _, tt := range []struct{ tag, want string }{
		{`parameter:"Fields,kind=query,in=fields,value='id,name'"`, "id,name"},
		{`parameter:"Fields,kind=query,in=fields,value='a\\'b,c'"`, "a'b,c"},
		{`parameter:"Fields,kind=query,in=fields,value='a\\\\b,c'"`, `a\b,c`},
	} {
		t.Run(tt.want, func(t *testing.T) {
			spec, found, err := bindly.BindingSpecFromField(reflect.StructField{Name: "Fields", Type: reflect.TypeOf(""), Tag: reflect.StructTag(tt.tag)})
			if err != nil || !found || spec.DefaultValue != tt.want {
				t.Fatalf("value %#v found %v error %v", spec.DefaultValue, found, err)
			}
		})
	}
}

func TestSafeDynamicStructBinding(t *testing.T) {
	type filters struct{ IDs *[]int }
	targetType := reflect.StructOf([]reflect.StructField{{Name: "Filters", Type: reflect.TypeOf(filters{}), Tag: `bind:"kind=value,in=filters"`}})
	ids := []int{13}
	injector, err := bindly.NewInjector(bindly.WithProviders(values.New("value", map[string]any{"filters": filters{IDs: &ids}})))
	if err != nil {
		t.Fatal(err)
	}
	target := reflect.New(targetType)
	if err = injector.Bind(context.Background(), target.Interface()); err != nil {
		t.Fatal(err)
	}
	actual := target.Elem().Field(0).Interface().(filters)
	if actual.IDs == nil || !reflect.DeepEqual(*actual.IDs, ids) {
		t.Fatalf("dynamic struct corrupted: %+v", actual)
	}
}

func TestBindingAllocatesNilParents(t *testing.T) {
	type nested struct{ Name string }
	type target struct{ Nested *nested }
	injector, _ := bindly.NewInjector(bindly.WithProviders(values.New("query", map[string]any{"name": "Alice"})))
	plan, err := injector.CompilePlan(reflect.TypeOf(target{}), bindly.BindingSpec{Path: "Nested.Name", Location: state.Location{Kind: "query", In: "name"}})
	if err != nil {
		t.Fatal(err)
	}
	var actual target
	if err = injector.Bind(context.Background(), &actual, bindly.WithPlan(plan)); err != nil {
		t.Fatal(err)
	}
	if actual.Nested == nil || actual.Nested.Name != "Alice" {
		t.Fatalf("target %+v", actual)
	}
}

func TestProjectionWithoutClearsValueAndMarker(t *testing.T) {
	type has struct{ ID bool }
	type target struct {
		ID  int  `bind:"identifier,kind=query,in=id"`
		Has *has `setMarker:"true"`
	}
	injector, _ := bindly.NewInjector()
	plan, err := injector.CompilePlan(reflect.TypeOf(target{}))
	if err != nil {
		t.Fatal(err)
	}
	projection, err := plan.Projection()
	if err != nil {
		t.Fatal(err)
	}
	input := &target{ID: 7, Has: &has{ID: true}}
	copy, err := projection.Without(input, "identifier")
	if err != nil {
		t.Fatal(err)
	}
	actual := copy.(*target)
	if actual.ID != 0 || actual.Has.ID || input.ID != 7 || !input.Has.ID {
		t.Fatalf("copy %+v original %+v", actual, input)
	}
}

func TestPlanTreatsRecursiveCapabilitiesAsOpaqueFields(t *testing.T) {
	type input struct {
		Request *http.Request `bind:"kind=http_request"`
	}
	injector, err := bindly.NewInjector()
	if err != nil {
		t.Fatal(err)
	}
	plan, err := injector.CompilePlan(reflect.TypeOf(input{}))
	if err != nil {
		t.Fatal(err)
	}
	projection, err := plan.Projection()
	if err != nil {
		t.Fatal(err)
	}
	request, _ := http.NewRequest("GET", "http://example.com", nil)
	value, found, err := projection.Value(&input{Request: request}, "Request")
	if err != nil || !found || value != request {
		t.Fatalf("projection %v %v %v", value, found, err)
	}
}

func TestBindingPreservesRecursiveHTTPRequestCapability(t *testing.T) {
	request, _ := http.NewRequest("GET", "http://example.com", nil)
	injector, err := bindly.NewInjector(bindly.WithProviders(values.New("http_request", map[string]any{"": request})))
	if err != nil {
		t.Fatal(err)
	}
	var input struct {
		Request *http.Request `bind:"kind=http_request"`
	}
	if err = injector.Bind(context.Background(), &input); err != nil {
		t.Fatal(err)
	}
	if input.Request != request {
		t.Fatal("request pointer was not preserved")
	}
}

func TestExplicitEmptyPlanDisablesTaggedBinding(t *testing.T) {
	type input struct {
		ID int `parameter:"ID,kind=path,in=id,required=true"`
	}
	injector, err := bindly.NewInjector()
	if err != nil {
		t.Fatal(err)
	}
	empty := make([]bindly.BindingSpec, 0)
	plan, err := injector.CompilePlan(reflect.TypeOf(input{}), empty...)
	if err != nil {
		t.Fatal(err)
	}
	if err = injector.Bind(context.Background(), &input{}, bindly.WithPlan(plan)); err != nil {
		t.Fatalf("inactive binding restored from tags: %v", err)
	}
}
