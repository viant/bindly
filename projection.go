package bindly

import (
	"fmt"
	"reflect"
	"strings"
)

// ProjectionField adds names for a destination path that is not already
// represented by a binding, or extends the names of a bound path.
type ProjectionField struct {
	Path  string
	Names []string
}

// ValueResolver resolves one projected name from a bound target.
type ValueResolver func(name string) (value any, ok bool, err error)

// Projection is an immutable, reusable name-to-destination projection for one
// Plan target type. It reads values only; providers and binding remain Plan
// concerns.
type Projection struct {
	targetType reflect.Type
	fields     map[string]projectedField
}

type projectedField struct {
	path  string
	index []int
}

// TargetType returns the exact struct type accepted by this projection.
func (p *Projection) TargetType() reflect.Type {
	if p == nil {
		return nil
	}
	return p.targetType
}

// Projection compiles value access from the same destination selectors owned
// by the Plan. Bound fields automatically expose their destination path,
// logical name, and physical source name. Explicit fields can add aliases or
// project declared fields that are initialized outside provider binding.
func (p *Plan) Projection(fields ...ProjectionField) (*Projection, error) {
	if p == nil || p.targetType == nil {
		return nil, fmt.Errorf("binding plan is required for value projection")
	}
	result := &Projection{
		targetType: p.targetType,
		fields:     make(map[string]projectedField),
	}
	for _, group := range p.bindingType.Bindings {
		for _, binding := range group {
			if binding == nil {
				continue
			}
			path := binding.destinationPath()
			names := []string{path, binding.Name}
			if !strings.EqualFold(strings.TrimSpace(binding.Kind()), "param") {
				names = append(names, binding.In())
			}
			if err := result.add(path, binding.index, names); err != nil {
				return nil, err
			}
		}
	}
	compiler := planCompiler{targetType: p.targetType}
	for _, field := range fields {
		path := strings.TrimSpace(field.Path)
		if path == "" {
			return nil, fmt.Errorf("projection destination path is required")
		}
		_, index, err := compiler.field(path)
		if err != nil {
			return nil, err
		}
		names := append([]string{path}, field.Names...)
		if err := result.add(path, index, names); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (p *Projection) add(path string, index []int, names []string) error {
	field := projectedField{path: path, index: append([]int(nil), index...)}
	for _, name := range names {
		key := projectionName(name)
		if key == "" {
			continue
		}
		if existing, ok := p.fields[key]; ok && !reflect.DeepEqual(existing.index, field.index) {
			return fmt.Errorf("projection name %q targets both %s and %s", name, existing.path, path)
		}
		p.fields[key] = field
	}
	return nil
}

// Value returns one projected value from a target compatible with the Plan.
func (p *Projection) Value(target any, name string) (any, bool, error) {
	if p == nil {
		return nil, false, fmt.Errorf("value projection is required")
	}
	field, ok := p.fields[projectionName(name)]
	if !ok {
		return nil, false, nil
	}
	value, err := projectionTarget(target, p.targetType)
	if err != nil {
		return nil, false, err
	}
	value, nilValue, err := projectedValue(value, field.index)
	if err != nil {
		return nil, false, fmt.Errorf("read projected value %s: %w", field.path, err)
	}
	if nilValue {
		return nil, true, nil
	}
	if !value.CanInterface() {
		return nil, false, fmt.Errorf("projected value %s is not accessible", field.path)
	}
	return value.Interface(), true, nil
}

// Resolver binds this immutable projection to one invocation target.
func (p *Projection) Resolver(target any) ValueResolver {
	return func(name string) (any, bool, error) {
		return p.Value(target, name)
	}
}

func projectionTarget(target any, targetType reflect.Type) (reflect.Value, error) {
	value := reflect.ValueOf(target)
	for value.IsValid() && (value.Kind() == reflect.Pointer || value.Kind() == reflect.Interface) {
		if value.IsNil() {
			return reflect.Value{}, fmt.Errorf("projection target must be non-nil")
		}
		value = value.Elem()
	}
	if !value.IsValid() || value.Type() != targetType {
		return reflect.Value{}, fmt.Errorf("projection target must resolve to %s, got %T", targetType, target)
	}
	return value, nil
}

func projectedValue(value reflect.Value, index []int) (reflect.Value, bool, error) {
	for _, position := range index {
		for value.IsValid() && (value.Kind() == reflect.Pointer || value.Kind() == reflect.Interface) {
			if value.IsNil() {
				return reflect.Value{}, true, nil
			}
			value = value.Elem()
		}
		if !value.IsValid() || value.Kind() != reflect.Struct || position < 0 || position >= value.NumField() {
			return reflect.Value{}, false, fmt.Errorf("invalid destination selector")
		}
		value = value.Field(position)
	}
	return value, false, nil
}

func projectionName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}
