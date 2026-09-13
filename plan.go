package bindly

import (
	"context"
	"fmt"
	"github.com/viant/bindly/internal/field"
	"github.com/viant/bindly/state"
	xshape "github.com/viant/x/shape"
	"reflect"
	"strings"

	"github.com/viant/tagly/tags"
)

// Plan contains only immutable target metadata; providers are resolved per bind.
type Plan struct {
	target   reflect.Type
	bindings []BindingSpec
	fields   map[string]field.Access
}

func (i *Injector) CompilePlan(target reflect.Type, specs ...BindingSpec) (*Plan, error) {
	for target != nil && target.Kind() == reflect.Pointer {
		target = target.Elem()
	}
	if target == nil || target.Kind() != reflect.Struct {
		return nil, fmt.Errorf("binding target must be a struct")
	}
	plan := &Plan{target: target, fields: map[string]field.Access{}}
	if specs == nil {
		for _, field := range reflect.VisibleFields(target) {
			if !field.IsExported() {
				continue
			}
			spec, found, err := bindingSpecFromField(field, i.bindingTag, i.bindingTag == bindingTag)
			if err != nil {
				return nil, err
			}
			if found {
				specs = append(specs, spec)
			} else if field.Type.Kind() == reflect.Interface {
				specs = append(specs, BindingSpec{Path: field.Name, Name: field.Name, Location: state.Location{Kind: i.interfaceKind, In: field.Type.String()}})
			}
		}
	}
	seen := map[string]bool{}
	for _, spec := range specs {
		if seen[spec.Path] {
			return nil, fmt.Errorf("duplicate binding path %s", spec.Path)
		}
		seen[spec.Path] = true
		resolved, err := xshape.Linked(target).StructField(spec.Path)
		if err != nil {
			return nil, err
		}
		plan.fields[spec.Path] = field.Access{Index: resolved.Index, Type: resolved.Type}
		if err := spec.compileRecordCount(resolved.Type); err != nil {
			return nil, fmt.Errorf("binding %s: %w", spec.Path, err)
		}
		if spec.MarkerField == "" {
			for _, candidate := range reflect.VisibleFields(target) {
				if candidate.Tag.Get("setMarker") == "true" {
					leaf := spec.Path
					if index := strings.LastIndex(leaf, "."); index >= 0 {
						leaf = leaf[index+1:]
					}
					path := candidate.Name + "." + leaf
					if marker, err := xshape.Linked(target).StructField(path); err == nil && marker.Type.Kind() == reflect.Bool {
						spec.MarkerField = path
						break
					}
				}
			}
		}
		if spec.MarkerField != "" {
			marker, err := xshape.Linked(target).StructField(spec.MarkerField)
			if err != nil {
				return nil, err
			}
			if marker.Type.Kind() != reflect.Bool {
				return nil, fmt.Errorf("binding marker %s must be bool", spec.MarkerField)
			}
			plan.fields[spec.MarkerField] = field.Access{Index: marker.Index, Type: marker.Type}
		}
		if spec.Location.Kind == "" {
			return nil, fmt.Errorf("binding %s requires a source kind", spec.Path)
		}
		if spec.Required != nil {
			v := *spec.Required
			spec.Required = &v
		}
		if spec.Cacheable != nil {
			v := *spec.Cacheable
			spec.Cacheable = &v
		}
		if spec.Transformer == nil {
			if raw, ok := resolved.Tag.Lookup(i.xformTag); ok {
				name, arguments := tags.Values(raw).Name()
				factory, ok := i.transformers.Lookup(name)
				if !ok {
					return nil, fmt.Errorf("failed to lookup transformer: %s", name)
				}
				transformer, err := factory.Create(context.Background(), arguments, resolved.Type, nil)
				if err != nil {
					return nil, err
				}
				spec.Transformer = transformer
			}
		}
		plan.bindings = append(plan.bindings, spec)
	}
	return plan, nil
}

func (p *Plan) TargetType() reflect.Type {
	if p == nil {
		return nil
	}
	return p.target
}
