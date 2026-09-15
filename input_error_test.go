package bindly_test

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/viant/bindly"
	"github.com/viant/bindly/input"
	"github.com/viant/bindly/locator"
	"github.com/viant/bindly/provider/body"
	"github.com/viant/bindly/provider/request"
	"github.com/viant/bindly/provider/values"
	"github.com/viant/bindly/state"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestExternalConversionStatusAndPrivateCause(t *testing.T) {
	for _, kind := range []string{"query", "path", "header", "cookie", "form", "body", "component", "data_view"} {
		for _, code := range []int{0, 422} {
			t.Run(kind+strconv.Itoa(code), func(t *testing.T) {
				source, _ := body.New([]byte(`{"id":"PRIVATE invalid integer"}`), "application/json", nil)
				providers := request.NewValues(request.WithQuery(url.Values{"id": {"PRIVATE invalid integer"}}), request.WithPathParams(map[string]string{"id": "PRIVATE invalid integer"}), request.WithHeaders(http.Header{"Id": {"PRIVATE invalid integer"}}), request.WithCookies(map[string]string{"id": "PRIVATE invalid integer"}), request.WithForm(url.Values{"id": {"PRIVATE invalid integer"}}), request.WithBodySource(source)).Providers()
				if kind == "component" || kind == "data_view" {
					providers = []locator.Provider{values.New(kind, map[string]any{"id": "PRIVATE invalid integer"})}
				}
				injector, err := bindly.NewInjector(bindly.WithProviders(providers...))
				if err != nil {
					t.Fatal(err)
				}
				target := struct{ ID int }{}
				plan, err := injector.CompilePlan(reflect.TypeOf(target), bindly.BindingSpec{Path: "ID", Location: state.Location{Kind: kind, In: "id"}, ErrorCode: code})
				if err != nil {
					t.Fatal(err)
				}
				err = injector.Bind(context.Background(), &target, bindly.WithPlan(plan))
				var binding *bindly.BindingError
				var conversion *strconv.NumError
				var client *input.Error
				var jsonConversion *json.UnmarshalTypeError
				if !errors.As(err, &binding) || (!errors.As(err, &conversion) && !errors.As(err, &jsonConversion)) {
					t.Fatalf("cause lost: %v", err)
				}
				external := kind != "component" && kind != "data_view"
				if errors.As(err, &client) != external {
					t.Fatalf("internal provider misclassified: %v", err)
				}
				want := code
				if external && want == 0 {
					want = 400
				}
				if binding.StatusCode() != want {
					t.Fatalf("status=%d want=%d", binding.StatusCode(), want)
				}
				if external && strings.Contains(err.Error(), "PRIVATE") {
					t.Fatal("public conversion detail")
				}
			})
		}
	}
}

type invalidTransformer struct{}

func (invalidTransformer) Transform(context.Context, locator.Resolver, interface{}) (interface{}, error) {
	return "PRIVATE bad transformer output", nil
}
func TestTransformerOutputFailureRemainsInternal(t *testing.T) {
	providers := request.NewValues(request.WithQuery(url.Values{"id": {"1"}})).Providers()
	injector, err := bindly.NewInjector(bindly.WithProviders(providers...))
	if err != nil {
		t.Fatal(err)
	}
	target := struct{ ID int }{}
	plan, err := injector.CompilePlan(reflect.TypeOf(target), bindly.BindingSpec{Path: "ID", Location: state.Location{Kind: "query", In: "id"}, Transformer: invalidTransformer{}})
	if err != nil {
		t.Fatal(err)
	}
	err = injector.Bind(context.Background(), &target, bindly.WithPlan(plan))
	var binding *bindly.BindingError
	var client *input.Error
	if !errors.As(err, &binding) || errors.As(err, &client) || binding.StatusCode() != 0 {
		t.Fatalf("transformer defect became client error: %v", err)
	}
}
