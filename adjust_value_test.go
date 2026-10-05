package bindly

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/viant/structology"
	"reflect"
	"testing"
)

// helper to build a selector for a synthetic struct field
func selectorForField(t *testing.T, fieldName string, fieldType reflect.Type) *structology.Selector {
	t.Helper()
	type dummy struct{}

	// build a synthetic struct type that has the requested field
	sType := reflect.StructOf([]reflect.StructField{{
		Name: fieldName,
		Type: fieldType,
	}})

	stateType := structology.NewStateType(reflect.TypeOf(&struct{ F interface{} }{}))
	// replace underlying reflect.Type so that RootSelectors uses our field type
	stateType = structology.NewStateType(reflect.New(sType).Type())
	selectors := stateType.RootSelectors()
	if len(selectors) == 0 {
		t.Fatalf("no selectors for synthetic type %v", sType)
	}
	return selectors[0]
}

func TestBindingContext_AdjustValue_ScalarAndPointer(t *testing.T) {
	type sample struct{
		Value string
	}

	testCases := []struct {
		name        string
		selectorTyp reflect.Type
		value       interface{}
		want        interface{}
		wantErr     bool
	}{
		{
			name:        "compatible types returned as-is",
			selectorTyp: reflect.TypeOf(""),
			value:       "abc",
			want:        "abc",
		},
		{
			name:        "convert non-pointer to pointer when selector expects pointer",
			selectorTyp: reflect.TypeOf((*string)(nil)),
			value:       "abc",
			want:        ptrTo("abc"),
		},
		{
			name:        "dereference pointer when selector expects value",
			selectorTyp: reflect.TypeOf(""),
			value:       ptrTo("xyz"),
			want:        "xyz",
		},
		{
			name:        "nil value returns nil without error",
			selectorTyp: reflect.TypeOf(""),
			value:       nil,
			want:        nil,
		},
		{
			name:        "incompatible pointer types produce error",
			selectorTyp: reflect.TypeOf((*int)(nil)),
			value:       "foo",
			wantErr:     true,
		},
	}

	ctx := &BindingContext[struct{}]{}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			selector := selectorForField(t, "Field", tc.selectorTyp)
			got, err := ctx.adjustValue(selector, tc.value)
			if tc.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestBindingContext_AdjustValue_Slices(t *testing.T) {
	type sample struct{}
	ctx := &BindingContext[sample]{}

	intType := reflect.TypeOf([]int{})
	ptrIntType := reflect.TypeOf([]*int{})

	testCases := []struct {
		name        string
		selectorTyp reflect.Type
		value       interface{}
		want        interface{}
		wantErr     bool
	}{
		{
			name:        "slice of ints to slice of ints (no-op)",
			selectorTyp: intType,
			value:       []int{1, 2, 3},
			want:        []int{1, 2, 3},
		},
		{
			name:        "slice of ints to slice of *int",
			selectorTyp: ptrIntType,
			value:       []int{1, 2},
			want:        []*int{ptrTo(1).(*int), ptrTo(2).(*int)},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			selector := selectorForField(t, "Field", tc.selectorTyp)
			got, err := ctx.adjustValue(selector, tc.value)
			if tc.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, fmt.Sprintf("%T", tc.want), fmt.Sprintf("%T", got))
		})
	}
}

// ptrTo is a small helper used only in tests.
func ptrTo[T any](v T) interface{} {
	return &v
}

