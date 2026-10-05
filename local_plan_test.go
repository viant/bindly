package bindly_test

import (
	"context"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/viant/bindly"
	"github.com/viant/bindly/locator"
	"github.com/viant/bindly/locator/buildin"
	requestprovider "github.com/viant/bindly/provider/request"
	"github.com/viant/bindly/state"
	"github.com/viant/bindly/xform"
	"github.com/viant/structology"
)

type sourceTypeProvider struct {
	requested reflect.Type
}

func (p *sourceTypeProvider) Kind() string  { return "source_type" }
func (p *sourceTypeProvider) Priority() int { return 0 }
func (p *sourceTypeProvider) Locate(*structology.State) locator.Locator {
	return &sourceTypeLocator{provider: p}
}

type sourceTypeLocator struct {
	provider *sourceTypeProvider
}

func (l *sourceTypeLocator) Kind() string { return "source_type" }
func (l *sourceTypeLocator) Value(_ context.Context, targetType reflect.Type, _ string) (interface{}, bool, error) {
	l.provider.requested = targetType
	return "red,green", true, nil
}

type sourceTypeTransformer struct {
	calls int
}

func (t *sourceTypeTransformer) Transform(_ context.Context, _ locator.Resolver, value interface{}) (interface{}, error) {
	t.calls++
	return strings.Split(value.(string), ","), nil
}

var _ xform.Transformer = (*sourceTypeTransformer)(nil)

func TestPlanBindUsesActiveRequestScope(t *testing.T) {
	type input struct {
		Name string
		IDs  []int
	}
	root, err := bindly.NewInjector()
	if err != nil {
		t.Fatalf("NewInjector() error = %v", err)
	}
	plan, err := root.CompilePlan(reflect.TypeOf(input{}),
		bindly.BindingSpec{Path: "Name", Location: state.Location{Kind: requestprovider.QueryKind, In: "name"}},
		bindly.BindingSpec{Path: "IDs", Location: state.Location{Kind: requestprovider.QueryKind, In: "id"}},
	)
	if err != nil {
		t.Fatalf("CompilePlan() error = %v", err)
	}

	for _, testCase := range []struct {
		name string
		want string
		ids  []string
	}{
		{name: "first", want: "parent", ids: []string{"1", "2"}},
		{name: "second", want: "child", ids: []string{"7", "9"}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/users", nil)
			requestScope, err := requestprovider.New(req, requestprovider.WithQuery(url.Values{
				"name": []string{testCase.want},
				"id":   testCase.ids,
			}))
			if err != nil {
				t.Fatalf("request.New() error = %v", err)
			}
			injector, err := root.ForScope(requestScope.Providers()...)
			if err != nil {
				t.Fatalf("ForScope() error = %v", err)
			}
			actual := &input{}
			if err := injector.Bind(context.Background(), actual, bindly.WithPlan(plan)); err != nil {
				t.Fatalf("Bind() error = %v", err)
			}
			if actual.Name != testCase.want {
				t.Fatalf("Name = %q, want %q", actual.Name, testCase.want)
			}
			if !reflect.DeepEqual(actual.IDs, stringInts(testCase.ids)) {
				t.Fatalf("IDs = %#v, want %#v", actual.IDs, stringInts(testCase.ids))
			}
		})
	}
}

func TestPlanRequestsSourceTypeBeforeTransformation(t *testing.T) {
	type input struct {
		Values []string
	}
	provider := &sourceTypeProvider{}
	transformer := &sourceTypeTransformer{}
	root, err := bindly.NewInjector(bindly.WithProviders(provider))
	if err != nil {
		t.Fatalf("NewInjector() error = %v", err)
	}
	plan, err := root.CompilePlan(reflect.TypeOf(input{}), bindly.BindingSpec{
		Path:        "Values",
		SourceType:  reflect.TypeOf(""),
		Location:    state.Location{Kind: provider.Kind()},
		Transformer: transformer,
	})
	if err != nil {
		t.Fatalf("CompilePlan() error = %v", err)
	}
	actual := &input{}
	if err := root.Bind(context.Background(), actual, bindly.WithPlan(plan)); err != nil {
		t.Fatalf("Bind() error = %v", err)
	}
	if provider.requested != reflect.TypeOf("") {
		t.Fatalf("provider requested type = %v, want string", provider.requested)
	}
	if transformer.calls != 1 {
		t.Fatalf("transformer calls = %d, want 1", transformer.calls)
	}
	if !reflect.DeepEqual(actual.Values, []string{"red", "green"}) {
		t.Fatalf("Values = %#v", actual.Values)
	}
}

func TestPlanBindRejectsNarrowIntegerOverflow(t *testing.T) {
	type input struct{ Limit int8 }
	root, err := bindly.NewInjector()
	if err != nil {
		t.Fatalf("NewInjector() error = %v", err)
	}
	plan, err := root.CompilePlan(reflect.TypeOf(input{}), bindly.BindingSpec{
		Path:     "Limit",
		Location: state.Location{Kind: requestprovider.QueryKind, In: "limit"},
	})
	if err != nil {
		t.Fatalf("CompilePlan() error = %v", err)
	}
	req := httptest.NewRequest("GET", "/?limit=128", nil)
	requestScope, err := requestprovider.New(req)
	if err != nil {
		t.Fatalf("request.New() error = %v", err)
	}
	injector, err := root.ForScope(requestScope.Providers()...)
	if err != nil {
		t.Fatalf("ForScope() error = %v", err)
	}
	if err := injector.Bind(context.Background(), &input{}, bindly.WithPlan(plan)); err == nil {
		t.Fatal("expected overflow error")
	}
}

func TestCompilePlanValidation(t *testing.T) {
	type input struct{ Name string }
	root, err := bindly.NewInjector()
	if err != nil {
		t.Fatalf("NewInjector() error = %v", err)
	}
	tests := []struct {
		name string
		spec bindly.BindingSpec
	}{
		{name: "missing path", spec: bindly.BindingSpec{Location: state.Location{Kind: "query"}}},
		{name: "unknown path", spec: bindly.BindingSpec{Path: "Missing", Location: state.Location{Kind: "query"}}},
		{name: "missing kind", spec: bindly.BindingSpec{Path: "Name"}},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			if _, err := root.CompilePlan(reflect.TypeOf(input{}), testCase.spec); err == nil {
				t.Fatal("expected plan validation error")
			}
		})
	}
}

func TestExplicitStatePlanRequiresSource(t *testing.T) {
	type input struct{ Name string }
	root, err := bindly.NewInjector(bindly.WithProviders(buildin.Struct("state", "", 1)))
	if err != nil {
		t.Fatalf("NewInjector() error = %v", err)
	}
	plan, err := root.CompilePlan(reflect.TypeOf(input{}), bindly.BindingSpec{
		Path: "Name", Location: state.Location{Kind: "state", In: "Name"},
	})
	if err != nil {
		t.Fatalf("CompilePlan() error = %v", err)
	}
	err = root.Bind(context.Background(), &input{}, bindly.WithPlan(plan))
	if err == nil || !strings.Contains(err.Error(), "use WithSource") {
		t.Fatalf("Bind() error = %v, want missing source guidance", err)
	}
	actual := &input{}
	if err := root.Bind(context.Background(), actual, bindly.WithPlan(plan), bindly.WithSource(&input{Name: "Ada"})); err != nil {
		t.Fatalf("Bind() with source error = %v", err)
	}
	if actual.Name != "Ada" {
		t.Fatalf("Name = %q, want Ada", actual.Name)
	}
}

func stringInts(values []string) []int {
	result := make([]int, len(values))
	for i, value := range values {
		for _, digit := range value {
			result[i] = result[i]*10 + int(digit-'0')
		}
	}
	return result
}
