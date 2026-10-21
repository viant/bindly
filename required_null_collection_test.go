package bindly_test

import (
	"context"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/viant/bindly"
	bindinput "github.com/viant/bindly/input"
	"github.com/viant/bindly/locator"
	"github.com/viant/bindly/provider/request"
	"github.com/viant/bindly/provider/values"
	"github.com/viant/bindly/state"
)

type collectionNullInput struct {
	Rows  []int
	After string
	Has   *struct{ Rows, After bool } `setMarker:"true"`
}
type collectionNullTransformer struct{ calls *int }

func (c collectionNullTransformer) Transform(_ context.Context, _ locator.Resolver, v any) (any, error) {
	*c.calls++
	return v, nil
}

func TestRequiredBodyCollectionPresenceDiagnostic(t *testing.T) {
	for _, tc := range []struct {
		name, raw, want string
		accepted        bool
	}{
		{"noBytes", "", "parameter Rows is required", false},
		{"emptyObject", `{}`, "parameter Rows is required", false},
		{"unrelated", `{"other":null}`, "parameter Rows is required", false},
		{"presentNull", `{"data":null}`, "parameter Rows value is required but no data was found", false},
		{"emptyArray", `{"data":[]}`, "parameter Rows value is required but no data was found", false},
		{"nonemptyArray", `{"data":[7]}`, "", true},
		{"malformed", `{"data":`, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/", strings.NewReader(tc.raw))
			req.Header.Set("Content-Type", "application/json")
			source, err := request.New(req)
			require.NoError(t, err)
			defer source.Close()
			inj, err := bindly.NewInjector(bindly.WithProviders(source.Providers()...), bindly.WithProviders(values.New("generator", map[string]any{"later": "changed"})))
			require.NoError(t, err)
			required := true
			plan, err := inj.CompilePlan(reflect.TypeFor[collectionNullInput](), bindly.BindingSpec{Path: "Rows", Name: "Rows", Location: state.Location{Kind: "body", In: "data"}, Required: &required}, bindly.BindingSpec{Path: "After", Location: state.Location{Kind: "generator", In: "later"}})
			require.NoError(t, err)
			out := collectionNullInput{Rows: []int{99}, After: "unchanged"}
			observed := 0
			err = inj.Bind(t.Context(), &out, bindly.WithPlan(plan), bindly.WithBindingObserver(func(context.Context, bindly.BindingEvent) error { observed++; return nil }))
			if tc.accepted {
				require.NoError(t, err)
				require.Equal(t, []int{7}, out.Rows)
				require.Equal(t, "changed", out.After)
				return
			}
			require.Error(t, err)
			var binding *bindly.BindingError
			require.ErrorAs(t, err, &binding)
			require.Equal(t, 400, binding.StatusCode())
			require.Equal(t, "Rows", binding.Path)
			if tc.want != "" {
				require.Equal(t, tc.want, err.Error())
			} else if tc.name == "malformed" {
				require.Equal(t, "Bad Request", err.Error())
			}
			if tc.name == "presentNull" || tc.name == "emptyArray" {
				_, absence := binding.RequiredAbsenceMessage()
				require.False(t, absence)
				var inputErr *bindinput.Error
				require.ErrorAs(t, err, &inputErr)
				require.Equal(t, tc.want, inputErr.Cause.Error())
			}
			require.Equal(t, []int{99}, out.Rows)
			require.Equal(t, "unchanged", out.After)
			require.Nil(t, out.Has)
			require.Zero(t, observed)
		})
	}
}

func TestRequiredNullCollectionStatusMessageAndCountPriority(t *testing.T) {
	one, zero := 1, 0
	for _, tc := range []struct {
		name, message, diagnostic string
		min, expected, max        *int
	}{
		{name: "required", diagnostic: "parameter Rows value is required but no data was found"},
		{name: "minimum", min: &one, diagnostic: "expected at least 1 records, but had 0"},
		{name: "expected", expected: &one, diagnostic: "expected 1 records, but had 0"},
		{name: "maximum", max: &zero, diagnostic: "parameter Rows value is required but no data was found"},
		{name: "literal", message: "authored invalid body", diagnostic: "parameter Rows value is required but no data was found"},
		{name: "interpolation", message: "invalid: ${error}", diagnostic: "parameter Rows value is required but no data was found"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/", strings.NewReader(`{"data":null}`))
			req.Header.Set("Content-Type", "application/json")
			scope, err := request.New(req)
			require.NoError(t, err)
			defer scope.Close()
			inj, err := bindly.NewInjector(bindly.WithProviders(scope.Providers()...))
			require.NoError(t, err)
			required := true
			plan, err := inj.CompilePlan(reflect.TypeFor[collectionNullInput](), bindly.BindingSpec{Path: "Rows", Name: "Rows", Location: state.Location{Kind: "body", In: "data"}, Required: &required, ErrorCode: 422, ErrorMessage: tc.message, MinAllowedRecords: tc.min, ExpectedReturned: tc.expected, MaxAllowedRecords: tc.max})
			require.NoError(t, err)
			err = inj.Bind(t.Context(), new(collectionNullInput), bindly.WithPlan(plan))
			var binding *bindly.BindingError
			require.ErrorAs(t, err, &binding)
			require.Equal(t, 422, binding.StatusCode())
			var inputErr *bindinput.Error
			require.ErrorAs(t, err, &inputErr)
			require.Equal(t, tc.diagnostic, inputErr.Cause.Error())
			switch tc.name {
			case "literal":
				require.Equal(t, tc.message, err.Error())
			case "interpolation":
				require.Equal(t, "invalid: "+tc.diagnostic, err.Error())
			default:
				require.Equal(t, tc.diagnostic, err.Error())
			}
		})
	}
}

func TestRequiredNullCollectionUsesSourceTypeWithoutCodecExecution(t *testing.T) {
	for _, tc := range []struct {
		name, kind     string
		target, source reflect.Type
		codec, noData  bool
	}{
		{"slice", "body", reflect.TypeFor[[]int](), nil, false, true},
		{"array", "body", reflect.TypeFor[[2]int](), nil, false, true},
		{"pointerSlice", "body", reflect.TypeFor[*[]int](), nil, false, true},
		{"knownCodecCollection", "body", reflect.TypeFor[string](), reflect.TypeFor[[]int](), true, true},
		{"unknownCodecSource", "body", reflect.TypeFor[[]int](), nil, true, false},
		{"scalarCodecSource", "body", reflect.TypeFor[[]int](), reflect.TypeFor[string](), true, false},
		{"scalar", "body", reflect.TypeFor[string](), nil, false, false},
		{"map", "body", reflect.TypeFor[map[string]int](), nil, false, false},
		{"struct", "body", reflect.TypeFor[*struct{ ID int }](), nil, false, false},
		{"nonbody", "header", reflect.TypeFor[[]int](), nil, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			target := reflect.StructOf([]reflect.StructField{{Name: "Value", Type: tc.target}})
			required := true
			calls := 0
			b := bindly.BindingSpec{Path: "Value", Name: "Rows", Location: state.Location{Kind: tc.kind, In: "data"}, Required: &required, SourceType: tc.source}
			if tc.codec {
				b.Transformer = collectionNullTransformer{&calls}
			}
			inj, err := bindly.NewInjector(bindly.WithProviders(values.New(tc.kind, map[string]any{"data": nil})))
			require.NoError(t, err)
			plan, err := inj.CompilePlan(target, b)
			require.NoError(t, err)
			err = inj.Bind(t.Context(), reflect.New(target).Interface(), bindly.WithPlan(plan))
			var binding *bindly.BindingError
			require.ErrorAs(t, err, &binding)
			_, absence := binding.RequiredAbsenceMessage()
			require.Equal(t, !tc.noData, absence)
			require.Zero(t, calls)
		})
	}
}

func TestNullCollectionOptionalAndDefaultsPreserved(t *testing.T) {
	for _, tc := range []struct {
		name, raw    string
		required     bool
		defaultValue any
		want         []int
		absent       bool
	}{
		{name: "optionalAbsent", raw: `{}`},
		{name: "optionalNull", raw: `{"data":null}`},
		{name: "absentDefault", raw: `{}`, required: true, defaultValue: []int{7}, want: []int{7}},
		{name: "nullDoesNotUseDefault", raw: `{"data":null}`, required: true, defaultValue: []int{7}},
		{name: "absentNilDefault", raw: `{}`, required: true, defaultValue: []int(nil), absent: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/", strings.NewReader(tc.raw))
			req.Header.Set("Content-Type", "application/json")
			scope, err := request.New(req)
			require.NoError(t, err)
			defer scope.Close()
			inj, err := bindly.NewInjector(bindly.WithProviders(scope.Providers()...))
			require.NoError(t, err)
			plan, err := inj.CompilePlan(reflect.TypeFor[collectionNullInput](), bindly.BindingSpec{Path: "Rows", Name: "Rows", Location: state.Location{Kind: "body", In: "data"}, Required: &tc.required, DefaultValue: tc.defaultValue})
			require.NoError(t, err)
			out := new(collectionNullInput)
			err = inj.Bind(t.Context(), out, bindly.WithPlan(plan))
			if tc.required && tc.want == nil {
				var binding *bindly.BindingError
				require.ErrorAs(t, err, &binding)
				_, absent := binding.RequiredAbsenceMessage()
				require.Equal(t, tc.absent, absent)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, out.Rows)
		})
	}
}
