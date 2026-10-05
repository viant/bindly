package bindly_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/viant/bindly"
	"github.com/viant/bindly/locator"
	"github.com/viant/bindly/provider/values"
	"github.com/viant/bindly/state"
)

type firstRecordTransformer struct{ called *bool }

func (f firstRecordTransformer) Transform(_ context.Context, _ locator.Resolver, value any) (any, error) {
	*f.called = true
	rows := value.([]int)
	return rows[0], nil
}

func TestRecordCountMissingSingletonAndCodec(t *testing.T) {
	type record struct{ ID int }
	type singleton struct{ Row *record }
	one := 1
	for _, present := range []bool{false, true} {
		data := map[string]any{}
		if present {
			data["row"] = &record{ID: 1}
		}
		injector, err := bindly.NewInjector(bindly.WithProviders(values.New("view", data)))
		if err != nil {
			t.Fatal(err)
		}
		plan, err := injector.CompilePlan(reflect.TypeOf(singleton{}), bindly.BindingSpec{Path: "Row", Location: state.Location{Kind: "view", In: "row"}, ExpectedReturned: &one})
		if err != nil {
			t.Fatal(err)
		}
		var actual singleton
		err = injector.Bind(context.Background(), &actual, bindly.WithPlan(plan))
		if (err == nil) != present {
			t.Fatalf("present=%t error=%v", present, err)
		}
	}
	for _, rows := range [][]int{{1}, {1, 2}} {
		injector, err := bindly.NewInjector(bindly.WithProviders(values.New("view", map[string]any{"rows": rows})))
		if err != nil {
			t.Fatal(err)
		}
		type target struct{ First int }
		called := false
		plan, err := injector.CompilePlan(reflect.TypeOf(target{}), bindly.BindingSpec{Path: "First", Location: state.Location{Kind: "view", In: "rows"}, SourceType: reflect.TypeOf([]int{}), ExpectedReturned: &one, Transformer: firstRecordTransformer{&called}})
		if err != nil {
			t.Fatal(err)
		}
		var actual target
		err = injector.Bind(context.Background(), &actual, bindly.WithPlan(plan))
		valid := len(rows) == 1
		if (err == nil) != valid || called != valid {
			t.Fatalf("rows=%v called=%t error=%v", rows, called, err)
		}
	}
}

func TestRecordCountPolicies(t *testing.T) {
	one, two, three := 1, 2, 3
	required := true
	for _, tt := range []struct {
		name            string
		value           []int
		min, max, exact *int
		required        *bool
		invalid         bool
	}{
		{name: "minimum accepted", value: []int{1}, min: &one},
		{name: "minimum rejected", value: []int{}, min: &one, invalid: true},
		{name: "maximum accepted", value: []int{1, 2}, max: &two},
		{name: "maximum rejected", value: []int{1, 2, 3}, max: &two, invalid: true},
		{name: "exact accepted", value: []int{1, 2}, exact: &two},
		{name: "exact rejected", value: []int{1}, exact: &two, invalid: true},
		{name: "range accepted", value: []int{1, 2}, min: &one, max: &three},
		{name: "nil minimum rejected", min: &one, invalid: true},
		{name: "required empty rejected", value: []int{}, required: &required, invalid: true},
		{name: "optional empty accepted", value: []int{}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			type input struct{ Rows []int }
			injector, err := bindly.NewInjector(bindly.WithProviders(values.New("body", map[string]any{"rows": tt.value})))
			if err != nil {
				t.Fatal(err)
			}
			plan, err := injector.CompilePlan(reflect.TypeOf(input{}), bindly.BindingSpec{Path: "Rows", Name: "Rows", Location: state.Location{Kind: "body", In: "rows"}, MinAllowedRecords: tt.min, MaxAllowedRecords: tt.max, ExpectedReturned: tt.exact, Required: tt.required, ErrorCode: 422, ErrorMessage: "invalid row count"})
			if err != nil {
				t.Fatal(err)
			}
			actual := input{Rows: []int{99}}
			err = injector.Bind(context.Background(), &actual, bindly.WithPlan(plan))
			if tt.invalid {
				var bindingError *bindly.BindingError
				if !errors.As(err, &bindingError) || bindingError.StatusCode() != 422 || err.Error() != "invalid row count" {
					t.Fatalf("error=%v", err)
				}
				if !reflect.DeepEqual(actual.Rows, []int{99}) {
					t.Fatal("invalid value assigned")
				}
			} else if err != nil || !reflect.DeepEqual(actual.Rows, tt.value) {
				t.Fatalf("value=%v error=%v", actual.Rows, err)
			}
		})
	}
}

func TestRecordCountPlanValidationAndIsolation(t *testing.T) {
	one, two, negative := 1, 2, -1
	injector, err := bindly.NewInjector()
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name   string
		target reflect.Type
		spec   bindly.BindingSpec
	}{
		{"negative", reflect.TypeOf(struct{ Rows []int }{}), bindly.BindingSpec{MinAllowedRecords: &negative}},
		{"inverted", reflect.TypeOf(struct{ Rows []int }{}), bindly.BindingSpec{MinAllowedRecords: &two, MaxAllowedRecords: &one}},
		{"exact outside range", reflect.TypeOf(struct{ Rows []int }{}), bindly.BindingSpec{MinAllowedRecords: &two, ExpectedReturned: &one}},
		{"scalar", reflect.TypeOf(struct{ Rows int }{}), bindly.BindingSpec{MinAllowedRecords: &one}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tt.spec.Path = "Rows"
			tt.spec.Location = state.Location{Kind: "body", In: "rows"}
			if _, err := injector.CompilePlan(tt.target, tt.spec); err == nil {
				t.Fatal("invalid constraint accepted")
			}
		})
	}
	type input struct {
		Rows []int `bind:"kind=body,in=rows,minAllowedRecords=1,maxAllowedRecords=2,expectedReturned=1"`
	}
	spec, _, err := bindly.BindingSpecFromField(reflect.TypeOf(input{}).Field(0))
	if err != nil {
		t.Fatal(err)
	}
	plan, err := injector.CompilePlan(reflect.TypeOf(input{}), spec)
	if err != nil {
		t.Fatal(err)
	}
	*spec.MinAllowedRecords = 100
	*spec.MaxAllowedRecords = 0
	*spec.ExpectedReturned = 100
	scope, err := injector.ForScope(values.New("body", map[string]any{"rows": []int{1}}))
	if err != nil {
		t.Fatal(err)
	}
	var actual input
	if err := scope.Bind(context.Background(), &actual, bindly.WithPlan(plan)); err != nil {
		t.Fatal(err)
	}
}
