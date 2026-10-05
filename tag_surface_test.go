package bindly

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func TestBindTagCarriesOriginalParameterSurface(t *testing.T) {
	type target struct {
		Value string `bind:"logical,kind=test,in=source,when=enabled,scope=request,errorCode=422,errorMessage=invalid,dataType=string,cardinality=One,with=Aux,required=false,cacheable=false,async=true,uri=embed.sql,resource=assets:query.sql,value='fallback'"`
	}
	provider := &testProvider{kind: "test", priority: 1, loc: &fixedLocator{kind: "test", value: "resolved"}}
	injector, err := NewInjector(WithProviders(provider))
	if err != nil {
		t.Fatalf("NewInjector() error = %v", err)
	}
	actual := &target{}
	if err := injector.Bind(context.Background(), actual); err != nil {
		t.Fatalf("Bind() error = %v", err)
	}
	bindingType, ok := injector.bindingCache.Get(reflect.TypeOf(target{}))
	if !ok || len(bindingType.Bindings) != 1 || len(bindingType.Bindings[0]) != 1 {
		t.Fatalf("binding plan was not cached: %+v", bindingType)
	}
	binding := bindingType.Bindings[0][0]
	if binding.Name != "logical" || binding.Kind() != "test" || binding.In() != "source" {
		t.Fatalf("unexpected identity: %+v", binding)
	}
	if binding.When != "enabled" || binding.Scope != "request" || binding.ErrorCode != 422 || binding.ErrorMessage != "invalid" {
		t.Fatalf("unexpected control metadata: %+v", binding)
	}
	if binding.DataType != "string" || binding.Cardinality != "One" || binding.With != "Aux" || binding.URI != "embed.sql" {
		t.Fatalf("unexpected type metadata: %+v", binding)
	}
	if binding.IsRequired() || binding.IsCacheable() || !binding.Async || binding.ResourceRef != "assets:query.sql" {
		t.Fatalf("unexpected flags/resource metadata: %+v", binding)
	}
	if binding.DefaultValue != "fallback" || actual.Value != "resolved" {
		t.Fatalf("unexpected default/resolved values: default=%v actual=%q", binding.DefaultValue, actual.Value)
	}
}

type fixedLocator struct {
	kind  string
	value interface{}
}

func (l *fixedLocator) Kind() string { return l.kind }
func (l *fixedLocator) Value(context.Context, reflect.Type, string) (interface{}, bool, error) {
	return l.value, true, nil
}

func TestBindTagRejectsUnknownMetadata(t *testing.T) {
	type target struct {
		Value string `bind:"kind=test,in=value,unknown=true"`
	}
	provider := &testProvider{kind: "test", priority: 1, loc: &fixedLocator{kind: "test"}}
	injector, err := NewInjector(WithProviders(provider))
	if err != nil {
		t.Fatalf("NewInjector() error = %v", err)
	}
	if err := injector.Bind(context.Background(), &target{}); err == nil {
		t.Fatal("expected unsupported bind tag key error")
	}
}

func TestBindingSpecFromFieldUsesCanonicalTagParser(t *testing.T) {
	type target struct {
		Value int `bind:"limit,kind=query,in=limit,required,cacheable=false"`
	}
	field, _ := reflect.TypeOf(target{}).FieldByName("Value")
	spec, ok, err := BindingSpecFromField(field)
	if err != nil {
		t.Fatalf("BindingSpecFromField() error = %v", err)
	}
	if !ok || spec.Path != "Value" || spec.Name != "limit" || spec.Location.Kind != "query" || spec.Location.In != "limit" {
		t.Fatalf("unexpected spec: %+v, ok=%v", spec, ok)
	}
	if spec.Required == nil || !*spec.Required || spec.Cacheable == nil || *spec.Cacheable {
		t.Fatalf("unexpected flags: required=%v cacheable=%v", spec.Required, spec.Cacheable)
	}
}

func TestParameterTagUsesCanonicalBindingParser(t *testing.T) {
	type target struct {
		Value string `parameter:"logical,kind=test,in=source,required"`
	}
	provider := &testProvider{kind: "test", priority: 1, loc: &fixedLocator{kind: "test", value: "resolved"}}
	injector, err := NewInjector(WithProviders(provider))
	if err != nil {
		t.Fatalf("NewInjector() error = %v", err)
	}
	actual := &target{}
	if err := injector.Bind(context.Background(), actual); err != nil {
		t.Fatalf("Bind() error = %v", err)
	}
	if actual.Value != "resolved" {
		t.Fatalf("unexpected resolved value %q", actual.Value)
	}
	field, _ := reflect.TypeOf(target{}).FieldByName("Value")
	spec, ok, err := BindingSpecFromField(field)
	if err != nil {
		t.Fatalf("BindingSpecFromField() error = %v", err)
	}
	if !ok || spec.Name != "logical" || spec.Location.Kind != "test" || spec.Location.In != "source" || spec.Required == nil || !*spec.Required {
		t.Fatalf("unexpected parameter alias spec: %+v, ok=%v", spec, ok)
	}
}

func TestBindingTagsRejectAmbiguousAlias(t *testing.T) {
	type target struct {
		Value string `bind:"kind=test,in=bind" parameter:"kind=test,in=parameter"`
	}
	injector, err := NewInjector()
	if err != nil {
		t.Fatalf("NewInjector() error = %v", err)
	}
	if err := injector.Bind(context.Background(), &target{}); err == nil || !strings.Contains(err.Error(), "cannot both be declared") {
		t.Fatalf("expected ambiguous binding tag error, got %v", err)
	}
	field, _ := reflect.TypeOf(target{}).FieldByName("Value")
	if _, _, err := BindingSpecFromField(field); err == nil || !strings.Contains(err.Error(), "cannot both be declared") {
		t.Fatalf("expected ambiguous binding spec error, got %v", err)
	}
}
