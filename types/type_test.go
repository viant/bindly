package types

import (
	"reflect"
	"testing"
)

// TestNewTypeAndMethods exercises construction of Type and its helpers.
func TestNewTypeAndMethods(t *testing.T) {
	type sample struct {
		Field1 int
		Field2 string
	}

	tests := []struct {
		name      string
		typeInput reflect.Type
		options   []Option
		verify    func(t *testing.T, got *Type)
	}{
		{
			name:      "compiled struct type",
			typeInput: reflect.TypeOf(sample{}),
			verify: func(t *testing.T, got *Type) {
				if got == nil {
					t.Fatalf("expected non-nil Type")
				}
				if got.CompiledType == nil {
					t.Fatalf("expected CompiledType to be set")
				}
				if got.GeneratedType != nil {
					t.Fatalf("expected GeneratedType to be nil for compiled type")
				}
				if got.Type() != reflect.TypeOf(sample{}) {
					t.Errorf("Type() = %v, want %v", got.Type(), reflect.TypeOf(sample{}))
				}
				if got.FullName() == "" {
					t.Errorf("expected FullName to be non-empty")
				}
				// ElementType should be nil for non-slice/array
				if got.ElementType() != nil {
					t.Errorf("ElementType() = %v, want nil", got.ElementType())
				}
			},
		},
		{
			name:      "pointer to slice element type",
			typeInput: reflect.TypeOf(&[]int{}),
			verify: func(t *testing.T, got *Type) {
				if got == nil {
					t.Fatalf("expected non-nil Type")
				}
				// Underlying type used for ElementType should be int
				if elem := got.ElementType(); elem != reflect.TypeOf(int(0)) {
					t.Errorf("ElementType() = %v, want %v", elem, reflect.TypeOf(int(0)))
				}
			},
		},
		{
			name:      "options override fields",
			typeInput: reflect.TypeOf(sample{}),
			options: []Option{
				WithName("CustomName"),
				WithPackage("custompkg"),
			},
			verify: func(t *testing.T, got *Type) {
				if got.Name != "CustomName" {
					t.Errorf("Name = %s, want CustomName", got.Name)
				}
				if got.Package != "custompkg" {
					t.Errorf("Package = %s, want custompkg", got.Package)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := NewType(tc.typeInput, tc.options...)
			if tc.verify != nil {
				// invoke the per‑case verifier with the constructed Type
				// to keep the table‑driven structure simple and clear.
				// we intentionally avoid additional shadow variables here
				// to keep the code idiomatic and free from unused identifiers.
				co := tc
				co.verify(t, got)
			}
		})
	}
}

// TestIsGeneratedStruct verifies detection of generated structs from reflect.StructOf
// and via pointer/slice/map wrappers.
func TestIsGeneratedStruct(t *testing.T) {
	type named struct{}

	generated := reflect.StructOf([]reflect.StructField{{
		Name: "X",
		Type: reflect.TypeOf(1),
	}})

	tests := []struct {
		name string
		in   reflect.Type
		out  bool
	}{
		{"named struct", reflect.TypeOf(named{}), false},
		{"generated struct", generated, true},
		{"pointer to generated", reflect.PtrTo(generated), true},
		{"slice of generated", reflect.SliceOf(generated), true},
		{"map to generated", reflect.MapOf(reflect.TypeOf(""), generated), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isGeneratedStruct(tt.in)
			if got != tt.out {
				t.Errorf("isGeneratedStruct(%v) = %v, want %v", tt.in, got, tt.out)
			}
		})
	}
}

// TestToReflectTypeEnsuresSlicesMapsStructsAreWrapped verifies structural fields and methods.
func TestToReflectTypeEnsuresSlicesMapsStructsAreWrapped(t *testing.T) {
	type inner struct{
		A int
	}

	tests := []struct {
		name string
		in   reflect.Type
	}{
		{"nil type returns nil", nil},
		{"slice type", reflect.TypeOf([]inner{})},
		{"map type", reflect.TypeOf(map[string]inner{})},
		{"struct type", reflect.TypeOf(inner{})},
		{"pointer type", reflect.TypeOf(&inner{})},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toReflectType(tt.in)
			if tt.in == nil {
				if got != nil {
					t.Fatalf("expected nil, got %#v", got)
				}
				return
			}

			if got == nil {
				t.Fatalf("expected non-nil ReflectType for %v", tt.in)
			}
			// For pointer case we expect IsPtr to be true and underlying type
			// to match Elem().
			if tt.in.Kind() == reflect.Ptr {
				if !got.IsPtr {
					t.Errorf("IsPtr = false, want true for pointer input")
				}
				if got.Type != tt.in.Elem() {
					t.Errorf("Type = %v, want %v", got.Type, tt.in.Elem())
				}
			}
			// For slice and map ensure nested metadata is non-nil.
			switch tt.in.Kind() {
			case reflect.Slice, reflect.Array:
				if got.SliceType == nil || got.SliceType.ElementType == nil {
					t.Errorf("expected SliceType with ElementType for %v", tt.in)
				}
			case reflect.Map:
				if got.MapType == nil || got.MapType.ValueType == nil {
					t.Errorf("expected MapType with ValueType for %v", tt.in)
				}
			case reflect.Struct:
				if got.StructType == nil {
					t.Fatalf("expected StructType for %v", tt.in)
				}
				if len(got.StructType.Field) == 0 {
					t.Errorf("expected at least one Field on StructType for %v", tt.in)
				}
			}
		})
	}
}

// TestRegisterAndLookupType ensures registry wiring behaves as expected.
func TestRegisterAndLookupType(t *testing.T) {
	type sample struct{}

	typ := NewType(reflect.TypeOf(sample{}), WithName("sample"))
	RegisterType(typ)

	got, ok := LookupType("sample")
	if !ok {
		t.Fatalf("expected type to be found in registry")
	}
	if got != typ {
		t.Errorf("LookupType returned different instance")
	}
}
