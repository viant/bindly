package bindly_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/viant/bindly"
	"github.com/viant/bindly/locator/buildin"
	"github.com/viant/bindly/state"
)

type dynamicInput struct {
	ID   int
	Name string
}

type dynamicScope struct {
	Input *dynamicInput
}

type cacheScope struct {
	Query  map[string]interface{}
	Header map[string]interface{}
}

func TestDynamicContextAssign(t *testing.T) {
	injector, err := bindly.NewInjector(bindly.WithProviders(
		buildin.Struct("input", "Input", 1),
	))
	if err != nil {
		t.Fatal(err)
	}
	scope := bindly.WithDynamicState(injector, &dynamicScope{
		Input: &dynamicInput{ID: 7, Name: "abc"},
	})

	target := &dynamicInput{}
	err = scope.Assign(context.Background(), target, &state.Location{Kind: "input"})
	if err != nil {
		t.Fatalf("assign failed: %v", err)
	}

	assert.Equal(t, &dynamicInput{ID: 7, Name: "abc"}, target)
}

func TestDynamicContextValue(t *testing.T) {
	injector, err := bindly.NewInjector(bindly.WithProviders(
		buildin.Struct("input", "Input", 1),
	))
	if err != nil {
		t.Fatal(err)
	}
	scope := bindly.WithDynamicState(injector, &dynamicScope{
		Input: &dynamicInput{ID: 11, Name: "xyz"},
	})

	value, ok, err := scope.Value(context.Background(), &state.Location{Kind: "input"})
	if err != nil {
		t.Fatalf("value lookup failed: %v", err)
	}
	if !ok {
		t.Fatalf("expected input lookup to succeed")
	}

	actual, ok := value.(*dynamicInput)
	if !ok {
		t.Fatalf("unexpected lookup type %T", value)
	}
	assert.Equal(t, &dynamicInput{ID: 11, Name: "xyz"}, actual)
}

func TestDynamicContextAssignRejectsNonPointerTarget(t *testing.T) {
	injector, err := bindly.NewInjector(bindly.WithProviders(
		buildin.Struct("input", "Input", 1),
	))
	if err != nil {
		t.Fatal(err)
	}
	scope := bindly.WithDynamicState(injector, &dynamicScope{
		Input: &dynamicInput{ID: 3},
	})

	err = scope.Assign(context.Background(), dynamicInput{}, &state.Location{Kind: "input"})
	if err == nil {
		t.Fatalf("expected non-pointer assign to fail")
	}
}

func TestDynamicContextInjectCacheUsesSourceLocation(t *testing.T) {
	injector, err := bindly.NewInjector(bindly.WithProviders(
		buildin.Map("query", "Query", 1),
		buildin.Map("header", "Header", 1),
	))
	if err != nil {
		t.Fatal(err)
	}
	scope := bindly.WithDynamicState(injector, &cacheScope{
		Query:  map[string]interface{}{"id": 7},
		Header: map[string]interface{}{"id": true},
	})

	type fromQuery struct {
		ID int `bind:"kind=query,in=id,cacheable=true"`
	}
	type fromHeader struct {
		ID bool `bind:"kind=header,in=id,cacheable=true"`
	}

	queryTarget := &fromQuery{}
	if err := scope.Inject(context.Background(), queryTarget); err != nil {
		t.Fatalf("query inject failed: %v", err)
	}
	headerTarget := &fromHeader{}
	if err := scope.Inject(context.Background(), headerTarget); err != nil {
		t.Fatalf("header inject failed: %v", err)
	}

	assert.Equal(t, &fromQuery{ID: 7}, queryTarget)
	assert.Equal(t, &fromHeader{ID: true}, headerTarget)
}
