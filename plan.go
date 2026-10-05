package bindly

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/viant/bindly/state"
	"github.com/viant/bindly/xform"
)

// BindingSpec is registration-time binding metadata independent of struct
// tags. Datly and other compilers can build it directly from their own IR.
type BindingSpec struct {
	Path string
	// SourceType is the value type requested from the provider before any
	// transformer runs. Nil defaults to the destination field type.
	SourceType   reflect.Type
	Location     state.Location
	Name         string
	Scope        string
	When         string
	ErrorCode    int
	ErrorMessage string
	DataType     string
	Cardinality  string
	With         string
	URI          string
	ResourceRef  string
	Required     *bool
	Cacheable    *bool
	Async        bool
	DefaultValue interface{}
	Priority     int
	Extension    interface{}
	Transformer  xform.Transformer
}

// Plan is an immutable, reusable binding plan for one destination struct type.
// It contains selectors and metadata only; providers are always resolved from
// the active injector scope at bind time.
type Plan struct {
	targetType  reflect.Type
	bindingType *BindingType
}

// CompilePlan validates explicit binding metadata and resolves destination
// selectors once. The resulting plan is safe to reuse across request scopes.
func (i *Injector) CompilePlan(targetType reflect.Type, specs ...BindingSpec) (*Plan, error) {
	if i == nil {
		return nil, fmt.Errorf("injector is required")
	}
	targetType, err := planTargetType(targetType)
	if err != nil {
		return nil, err
	}
	return (&planCompiler{injector: i, targetType: targetType}).compile(specs)
}

type planCompiler struct {
	injector   *Injector
	targetType reflect.Type
}

func (c *planCompiler) compile(specs []BindingSpec) (*Plan, error) {
	bindings := make(Bindings, 0, len(specs))
	seen := map[string]bool{}
	for _, spec := range specs {
		path := strings.TrimSpace(spec.Path)
		if path == "" {
			return nil, fmt.Errorf("binding destination path is required")
		}
		if seen[path] {
			return nil, fmt.Errorf("duplicate binding destination path %q", path)
		}
		seen[path] = true
		if strings.TrimSpace(spec.Location.Kind) == "" {
			return nil, fmt.Errorf("binding kind is required for path %q", path)
		}
		field, index, err := c.field(path)
		if err != nil {
			return nil, err
		}
		location := spec.Location
		required := cloneBool(spec.Required)
		cacheable := cloneBool(spec.Cacheable)
		binding := &Binding{
			path:         path,
			index:        index,
			fieldType:    field.Type,
			sourceType:   spec.SourceType,
			location:     &location,
			Name:         spec.Name,
			Scope:        spec.Scope,
			When:         spec.When,
			ErrorCode:    spec.ErrorCode,
			ErrorMessage: spec.ErrorMessage,
			DataType:     spec.DataType,
			Cardinality:  spec.Cardinality,
			With:         spec.With,
			URI:          spec.URI,
			ResourceRef:  spec.ResourceRef,
			Required:     required,
			Cacheable:    cacheable,
			Async:        spec.Async,
			DefaultValue: spec.DefaultValue,
			Tag:          field.Tag,
			Extension:    spec.Extension,
			priority:     spec.Priority,
			transformer:  spec.Transformer,
		}
		if binding.sourceType == nil {
			binding.sourceType = field.Type
		}
		if binding.Name == "" {
			binding.Name = path
		}
		bindings = append(bindings, binding)
	}
	sort.SliceStable(bindings, func(i, j int) bool {
		return bindings[i].priority < bindings[j].priority
	})
	return &Plan{
		targetType: c.targetType,
		bindingType: &BindingType{
			Bindings: groupBindings(bindings),
		},
	}, nil
}

func cloneBool(value *bool) *bool {
	if value == nil {
		return nil
	}
	result := *value
	return &result
}

func (c *planCompiler) field(path string) (reflect.StructField, []int, error) {
	current := c.targetType
	var index []int
	parts := strings.Split(path, ".")
	for i, name := range parts {
		for current.Kind() == reflect.Ptr {
			current = current.Elem()
		}
		if current.Kind() != reflect.Struct {
			return reflect.StructField{}, nil, fmt.Errorf("binding destination path %q traverses non-struct %s", path, current)
		}
		field, ok := current.FieldByName(name)
		if !ok {
			return reflect.StructField{}, nil, fmt.Errorf("binding destination path %q was not found on %s", path, c.targetType)
		}
		index = append(index, field.Index...)
		current = field.Type
		if i == len(parts)-1 {
			return field, index, nil
		}
	}
	return reflect.StructField{}, nil, fmt.Errorf("binding destination path %q was not found on %s", path, c.targetType)
}

func groupBindings(bindings Bindings) []Bindings {
	if len(bindings) == 0 {
		return nil
	}
	result := []Bindings{{bindings[0]}}
	for _, binding := range bindings[1:] {
		last := result[len(result)-1]
		if last[0].priority != binding.priority {
			result = append(result, Bindings{binding})
			continue
		}
		result[len(result)-1] = append(last, binding)
	}
	return result
}

func planTargetType(targetType reflect.Type) (reflect.Type, error) {
	if targetType == nil {
		return nil, fmt.Errorf("binding target type is required")
	}
	for targetType.Kind() == reflect.Ptr {
		targetType = targetType.Elem()
	}
	if targetType.Kind() != reflect.Struct {
		return nil, fmt.Errorf("binding target must be a struct, got %s", targetType)
	}
	return targetType, nil
}
