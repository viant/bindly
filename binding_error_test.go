package bindly_test

import (
	"context"
	"errors"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/viant/bindly"
	requestprovider "github.com/viant/bindly/provider/request"
	"github.com/viant/bindly/state"
)

func TestBindingErrorPreservesAuthoredStatusAndMessage(t *testing.T) {
	type input struct {
		Age int
	}
	required := true
	root, err := bindly.NewInjector()
	if err != nil {
		t.Fatalf("NewInjector() error = %v", err)
	}
	plan, err := root.CompilePlan(reflect.TypeOf(input{}), bindly.BindingSpec{
		Path:         "Age",
		Name:         "Age",
		Location:     state.Location{Kind: requestprovider.QueryKind, In: "age"},
		Required:     &required,
		ErrorCode:    422,
		ErrorMessage: "invalid age: ${error}",
	})
	if err != nil {
		t.Fatalf("CompilePlan() error = %v", err)
	}
	scope, err := requestprovider.New(httptest.NewRequest("GET", "/", nil))
	if err != nil {
		t.Fatalf("request.New() error = %v", err)
	}
	injector, err := root.ForScope(scope.Providers()...)
	if err != nil {
		t.Fatalf("ForScope() error = %v", err)
	}
	err = injector.Bind(context.Background(), &input{}, bindly.WithPlan(plan))
	var bindingErr *bindly.BindingError
	if !errors.As(err, &bindingErr) {
		t.Fatalf("Bind() error = %T %v, want *bindly.BindingError", err, err)
	}
	if bindingErr.StatusCode() != 422 || bindingErr.Name != "Age" || bindingErr.Kind != requestprovider.QueryKind || bindingErr.In != "age" {
		t.Fatalf("BindingError = %+v", bindingErr)
	}
	if !strings.HasPrefix(err.Error(), "invalid age: missing required query value") {
		t.Fatalf("Bind() error = %q", err)
	}
}
