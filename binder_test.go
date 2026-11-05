package bindly_test

import (
	"context"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/viant/bindly"
	"github.com/viant/bindly/provider/request"
	"github.com/viant/bindly/state"
)

func TestBinderInterfacePreservesDefaultAndExplicitPlan(t *testing.T) {
	type input struct {
		Count int `bind:"kind=query,in=count"`
	}
	root, err := bindly.NewInjector()
	if err != nil {
		t.Fatal(err)
	}
	plan, err := root.CompilePlan(reflect.TypeOf(input{}), bindly.BindingSpec{Path: "Count", Name: "Count", Location: state.Location{Kind: "query", In: "alternate"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, query := range []struct {
		url             string
		direct, planned int
	}{{"/?count=3&alternate=8", 3, 8}, {"/?count=4&alternate=9", 4, 9}} {
		transport, err := request.New(httptest.NewRequest("GET", query.url, nil))
		if err != nil {
			t.Fatal(err)
		}
		scope, err := root.ForScope(transport.Providers()...)
		if err != nil {
			t.Fatal(err)
		}
		var binder bindly.Binder = scope
		var plain, explicit input
		if err = binder.Bind(context.Background(), &plain); err != nil {
			t.Fatal(err)
		}
		if err = binder.Bind(context.Background(), &explicit, bindly.WithPlan(plan)); err != nil {
			t.Fatal(err)
		}
		if plain.Count != query.direct || explicit.Count != query.planned {
			t.Fatalf("default=%d explicit=%d", plain.Count, explicit.Count)
		}
		transport.Close()
	}
}
