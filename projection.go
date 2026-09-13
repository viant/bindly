package bindly

import (
	"fmt"
	"github.com/viant/bindly/internal/field"
	xshape "github.com/viant/x/shape"
	"reflect"
	"strings"
)

type ValueResolver func(string) (any, bool, error)
type ProjectionField struct {
	Path  string
	Names []string
}
type Projection struct {
	target  reflect.Type
	paths   map[string]string
	markers map[string]string
	fields  map[string]field.Access
}

func (p *Plan) Projection(fields ...ProjectionField) (*Projection, error) {
	if p == nil {
		return nil, fmt.Errorf("binding plan is required")
	}
	result := &Projection{target: p.target, paths: map[string]string{}, markers: map[string]string{}, fields: map[string]field.Access{}}
	structural := map[string]string{}
	add := func(name, path string) error {
		name = strings.ToLower(strings.TrimSpace(name))
		if name == "" {
			return nil
		}
		if previous, ok := result.paths[name]; ok && previous != path {
			if structural[name] == previous {
				return nil
			}
			return fmt.Errorf("ambiguous input alias %q", name)
		}
		result.paths[name] = path
		resolved, err := xshape.Linked(p.target).StructField(path)
		if err != nil {
			return err
		}
		result.fields[path] = field.Access{Index: resolved.Index, Type: resolved.Type}
		return nil
	}
	structFields, err := xshape.Linked(p.target).Fields()
	if err != nil {
		return nil, err
	}
	for _, item := range structFields {
		if !item.Exported {
			continue
		}
		if err := add(item.Name, item.Name); err != nil {
			return nil, err
		}
		structural[strings.ToLower(item.Name)] = item.Name
	}
	for _, binding := range p.bindings {
		if binding.MarkerField != "" {
			result.markers[binding.Path] = binding.MarkerField
		}
		names := []string{binding.Name}
		if !strings.EqualFold(binding.Location.Kind, "param") {
			names = append(names, binding.Location.In)
		}
		for _, name := range names {
			if err := add(name, binding.Path); err != nil {
				return nil, err
			}
		}
	}
	for _, field := range fields {
		if _, err := xshape.Linked(p.target).StructField(field.Path); err != nil {
			return nil, fmt.Errorf("projection path %q is not defined", field.Path)
		}
		for _, name := range field.Names {
			if err := add(name, field.Path); err != nil {
				return nil, err
			}
		}
	}
	return result, nil
}
func (p *Projection) TargetType() reflect.Type {
	if p == nil {
		return nil
	}
	return p.target
}
func (p *Projection) Resolver(target any) ValueResolver {
	return func(name string) (any, bool, error) { return p.Value(target, name) }
}
func (p *Projection) Value(target any, name string) (any, bool, error) {
	if p == nil {
		return nil, false, nil
	}
	path, ok := p.paths[strings.ToLower(strings.TrimSpace(name))]
	if !ok {
		return nil, false, nil
	}
	actual := reflect.ValueOf(target)
	for actual.IsValid() && actual.Kind() == reflect.Pointer {
		if actual.IsNil() {
			return nil, false, nil
		}
		actual = actual.Elem()
	}
	if !actual.IsValid() || actual.Type() != p.target {
		return nil, false, fmt.Errorf("projection target must be %v", p.target)
	}
	value, found := p.fields[path].Value(reflect.ValueOf(target))
	if !found {
		return nil, false, nil
	}
	return value.Interface(), true, nil
}

func (p *Projection) Without(target any, names ...string) (any, error) {
	if p == nil {
		return nil, fmt.Errorf("input projection is required")
	}
	var paths []string
	seen := map[string]bool{}
	for _, name := range names {
		path, ok := p.paths[strings.ToLower(strings.TrimSpace(name))]
		if !ok {
			return nil, fmt.Errorf("input alias %q is not defined", name)
		}
		for _, candidate := range []string{path, p.markers[path]} {
			if candidate != "" && !seen[candidate] {
				seen[candidate] = true
				paths = append(paths, candidate)
			}
		}
	}
	return (xshape.Runtime{}).WithZeroFields(target, paths...)
}
