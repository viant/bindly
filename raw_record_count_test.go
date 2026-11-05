package bindly_test

import (
	"context"
	"encoding/json"
	"github.com/viant/bindly"
	"github.com/viant/bindly/locator"
	"github.com/viant/bindly/provider/body"
	"github.com/viant/bindly/state"
	"reflect"
	"testing"
)

type rawRecords struct{}

func (rawRecords) Transform(_ context.Context, _ locator.Resolver, value any) (any, error) {
	if value == nil {
		return nil, nil
	}
	var rows []struct{ ID int }
	err := json.Unmarshal(value.(json.RawMessage), &rows)
	return rows, err
}

type rawRecord struct{ called *bool }

func (r rawRecord) Transform(context.Context, locator.Resolver, any) (any, error) {
	*r.called = true
	return &struct{ ID int }{}, nil
}
func TestRawJSONCountsDecodedRecordsAndPreservesRequiredNull(t *testing.T) {
	for _, tc := range []struct {
		raw   string
		valid bool
	}{{`[{"ID":123456789}]`, true}, {`[{"ID":1},{"ID":2}]`, false}} {
		type target struct{ Rows []struct{ ID int } }
		source, _ := body.New([]byte(tc.raw), "application/json", nil)
		root, _ := bindly.NewInjector(bindly.WithProviders(source))
		max := 1
		plan, err := root.CompilePlan(reflect.TypeFor[target](), bindly.BindingSpec{Path: "Rows", Name: "Rows", Location: state.Location{Kind: "body"}, SourceType: reflect.TypeFor[json.RawMessage](), Transformer: rawRecords{}, MaxAllowedRecords: &max})
		if err != nil {
			t.Fatal(err)
		}
		var result target
		err = root.Bind(t.Context(), &result, bindly.WithPlan(plan))
		if (err == nil) != tc.valid {
			t.Fatal(tc.raw, err)
		}
	}
	type target struct{ Row *struct{ ID int } }
	source, _ := body.New([]byte(`null`), "application/json", nil)
	root, _ := bindly.NewInjector(bindly.WithProviders(source))
	required := true
	called := false
	plan, err := root.CompilePlan(reflect.TypeFor[target](), bindly.BindingSpec{Path: "Row", Name: "Row", Location: state.Location{Kind: "body"}, SourceType: reflect.TypeFor[json.RawMessage](), Transformer: rawRecord{&called}, Required: &required})
	if err != nil {
		t.Fatal(err)
	}
	var result target
	if err = root.Bind(t.Context(), &result, bindly.WithPlan(plan)); err == nil || called {
		t.Fatal("required null bypassed admission", err, called)
	}
}
