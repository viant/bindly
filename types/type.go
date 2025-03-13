package types

import (
	"reflect"
	"strings"
)

type Type struct {
	Name    string
	Package string // import package alias, e.g. "mypkg"
	PkgPath string // PkgPath is the package path of the type, e.g. "github.com/repo/project/mypkg"

	CompiledType  *ReflectType
	GeneratedType *ReflectType // created by reflect.StructOf
	Embedder      Embedder     // if type has dependency on fs embedder
}

func (t *Type) Type() reflect.Type {
	if t.CompiledType != nil {
		return t.CompiledType.Type
	}
	return t.GeneratedType.Type
}

func (t *Type) ElementType() reflect.Type {
	rType := t.Type()
	if rType.Kind() == reflect.Ptr {
		rType = rType.Elem()
	}
	switch rType.Kind() {
	case reflect.Slice, reflect.Array:
		return rType.Elem()
	}
	return nil
}

func (t *Type) FullName() string {
	if t.PkgPath == "" {
		return t.Name
	}
	return t.PkgPath + "." + t.Name
}

type Field struct {
	// Name is the field name.
	Name string
	// PkgPath is the package path that qualifies a lower case (unexported)
	// field name. It is empty for upper case (exported) field names.
	PkgPath   string
	Type      ReflectType       // field type
	Tag       reflect.StructTag // field tag string
	Offset    uintptr           // offset within struct, in bytes
	Index     []int             // index sequence for Type.FieldByIndex
	Anonymous bool              // is an embedded field
}

type ReflectType struct {
	Type  reflect.Type // underlying type
	IsPtr bool
	*SliceType
	*MapType
	*StructType
}

type SliceType struct {
	ElementType *ReflectType
}

type MapType struct {
	KeyType   reflect.Type
	ValueType *ReflectType
}

type StructType struct {
	Method   []reflect.Method
	Field    []Field
	Embedder // if needed
}

// ----------------------------------------------------------------------------
// NewType
// ----------------------------------------------------------------------------

func NewType(rType reflect.Type, opts ...Option) *Type {
	ret := &Type{
		Name:    rType.Name(),
		PkgPath: rType.PkgPath(),
	}

	// Derive the package name from the last path component (best effort).
	if ret.PkgPath != "" {
		pathParts := strings.Split(ret.PkgPath, "/")
		ret.Package = pathParts[len(pathParts)-1]
	}

	// Mark whether it's a "generated" struct (i.e. no name, but Kind is struct)
	// vs. a normal named/compiled type.
	if isGeneratedStruct(rType) {
		ret.GeneratedType = toReflectType(rType)
	} else {
		ret.CompiledType = toReflectType(rType)
	}

	for _, opt := range opts {
		opt(ret)
	}

	return ret
}

// Helper: A struct is "generated" (by reflect.StructOf) if Kind=struct but has no name
func isGeneratedStruct(rType reflect.Type) bool {
	switch rType.Kind() {
	case reflect.Ptr:
		return isGeneratedStruct(rType.Elem())
	case reflect.Slice, reflect.Array:
		return isGeneratedStruct(rType.Elem())
	case reflect.Map:
		return isGeneratedStruct(rType.Elem())
	}
	return rType.Kind() == reflect.Struct && rType.Name() == ""
}

// ----------------------------------------------------------------------------
// toReflectType: converts reflect.Type to our ReflectType wrapper
// ----------------------------------------------------------------------------

func toReflectType(rType reflect.Type) *ReflectType {
	if rType == nil {
		return nil
	}
	rt := &ReflectType{
		Type: rType,
	}

	switch rType.Kind() {
	case reflect.Slice, reflect.Array:
		// Wrap as a slice-like type
		rt.SliceType = &SliceType{
			ElementType: toReflectType(rType.Elem()),
		}

	case reflect.Map:
		// Wrap as a map-like type
		rt.MapType = &MapType{
			KeyType:   rType.Key(),
			ValueType: toReflectType(rType.Elem()),
		}

	case reflect.Struct:
		// Wrap struct fields and methods
		rt.StructType = &StructType{
			Field:    makeStructFields(rType),
			Method:   makeStructMethods(rType),
			Embedder: nil, // or handle as needed
		}
	case reflect.Ptr:
		rt = toReflectType(rType.Elem())
		rt.IsPtr = true
	}
	return rt
}

// ----------------------------------------------------------------------------
// Helpers for Struct Fields and Methods
// ----------------------------------------------------------------------------

func makeStructFields(rType reflect.Type) []Field {
	fieldsCount := rType.NumField()
	fields := make([]Field, 0, fieldsCount)

	for i := 0; i < fieldsCount; i++ {
		sf := rType.Field(i)
		fields = append(fields, Field{
			Name:      sf.Name,
			PkgPath:   sf.PkgPath,
			Type:      *toReflectType(sf.Type),
			Tag:       sf.Tag,
			Offset:    sf.Offset,
			Index:     sf.Index,
			Anonymous: sf.Anonymous,
		})
	}
	return fields
}

func makeStructMethods(rType reflect.Type) []reflect.Method {
	methodsCount := rType.NumMethod()
	methods := make([]reflect.Method, 0, methodsCount)
	for i := 0; i < methodsCount; i++ {
		methods = append(methods, rType.Method(i))
	}
	return methods
}
