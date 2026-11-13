package bindly

import (
	"context"
	"crypto/sha256"
	"fmt"
	"github.com/viant/bindly/internal/field"
	"github.com/viant/bindly/state"
	xshape "github.com/viant/x/shape"
	"reflect"
	"strconv"
	"strings"

	"github.com/viant/tagly/tags"
)

// Plan contains only immutable target metadata; providers are resolved per bind.
type Plan struct {
	groups   []ResolutionGroupPlan
	target   reflect.Type
	bindings []BindingSpec
	fields   map[string]field.Access
}

func (i *Injector) CompilePlan(target reflect.Type, specs ...BindingSpec) (*Plan, error) {
	if i == nil {
		return nil, fmt.Errorf("injector is required")
	}
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
		resolved, err := resolvePlanField(target, spec.Path)
		if err != nil {
			return nil, err
		}
		plan.fields[spec.Path] = field.Access{Index: append([]int(nil), resolved.Index...), Type: resolved.Type}
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
					if marker, err := resolvePlanField(target, path); err == nil && marker.Type.Kind() == reflect.Bool {
						spec.MarkerField = path
						break
					}
				}
			}
		}
		if spec.MarkerField != "" {
			marker, err := resolvePlanField(target, spec.MarkerField)
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
		for _, metadata := range []*any{&spec.DefaultValue, &spec.Extension} {
			if *metadata != nil {
				cloned, err := (xshape.Runtime{}).CloneValue(*metadata)
				if err != nil {
					return nil, fmt.Errorf("binding %s metadata: %w", spec.Path, err)
				}
				*metadata = cloned
			}
		}

		if authored := resolved.Tag.Get("jsonx"); spec.Location.Kind == "body" && spec.Location.In == "" && hasObjectToSlicePolicy(authored) {
			tag := reflect.StructTag("jsonx:" + strconv.Quote(authored))
			if spec.BodyInputTag != "" && spec.BodyInputTag.Get("jsonx") != authored {
				return nil, fmt.Errorf("binding %s has conflicting body input metadata", spec.Path)
			}
			spec.BodyInputTag = tag
		}
		if err := spec.ValidateBodyInputTag(resolved.Type); err != nil {
			return nil, fmt.Errorf("binding %s: %w", spec.Path, err)
		}
		if err := spec.ValidatePresenceOnly(resolved.Type); err != nil {
			return nil, fmt.Errorf("binding %s: %w", spec.Path, err)
		}
		if err := spec.ValidateBodyNullPolicy(resolved.Type); err != nil {
			return nil, fmt.Errorf("binding %s: %w", spec.Path, err)
		}
		if metadata := spec.BodyRepresentation; metadata != nil {
			source := spec.SourceType
			if source == nil {
				source = resolved.Type
			}
			if spec.Location.Kind != "body" || spec.PresenceOnly || metadata.Original() != source {
				return nil, fmt.Errorf("binding %s body representation must match its native body source type", spec.Path)
			}
			if !metadata.Changed() {
				spec.BodyRepresentation = nil
			} else {
				spec.bodyRepresentationKey = fmt.Sprintf("%x", sha256.Sum256([]byte(metadata.Signature())))
			}
		}
		plan.bindings = append(plan.bindings, spec)
	}
	if err := plan.compileResolutionGroups(); err != nil {
		return nil, err
	}
	return plan, nil
}

func (p *Plan) TargetType() reflect.Type {
	if p == nil {
		return nil
	}
	return p.target
}

// Presence reads a binding's compiled presence marker. Inputs without a marker
// carry values unconditionally. Missing marker pointers represent absence.
func (p *Plan) Presence(target any, path string) (bool, error) {
	marker, err := p.presenceMarker(target, path)
	if err != nil {
		return false, err
	}
	if marker == "" {
		return true, nil
	}
	value, found := p.fields[marker].Value(reflect.ValueOf(target))
	if !found {
		return false, nil
	}
	return value.Bool(), nil
}

// SetPresence restores presence through the same compiled marker used by Bind.
func (p *Plan) SetPresence(target any, path string, present bool) error {
	marker, err := p.presenceMarker(target, path)
	if err != nil || marker == "" {
		return err
	}
	return p.fields[marker].Set(reflect.ValueOf(target), present)
}

func (p *Plan) presenceMarker(target any, path string) (string, error) {
	if p == nil {
		return "", fmt.Errorf("binding plan is required")
	}
	value := reflect.ValueOf(target)
	if !value.IsValid() || value.Kind() != reflect.Pointer || value.IsNil() || value.Elem().Type() != p.target {
		return "", fmt.Errorf("presence target must be *%s", p.target)
	}
	for _, binding := range p.bindings {
		if binding.Path == path {
			return binding.MarkerField, nil
		}
	}
	return "", fmt.Errorf("presence binding %s is not defined", path)
}

// resolvePlanField retains the complete selector index across nested parents.
func resolvePlanField(target reflect.Type, path string) (reflect.StructField, error) {
	var indexes []int
	var leaf reflect.StructField
	for _, name := range strings.Split(path, ".") {
		for target.Kind() == reflect.Pointer {
			target = target.Elem()
		}
		if target.Kind() != reflect.Struct {
			return leaf, fmt.Errorf("binding path %q traverses non-struct %s", path, target)
		}
		field, ok := target.FieldByName(name)
		if !ok || !field.IsExported() {
			return leaf, fmt.Errorf("binding path %q is not defined", path)
		}
		indexes = append(indexes, field.Index...)
		target = field.Type
		leaf = field
	}
	leaf.Index = indexes
	return leaf, nil
}

// ExplicitPresence reports suppliedness only from an actual compiled marker.
// Markerless inputs return hasMarker=false and cannot prove explicit null.
func (p *Plan) ExplicitPresence(target any, path string) (present, hasMarker bool, err error) {
	marker, err := p.presenceMarker(target, path)
	if err != nil || marker == "" {
		return false, false, err
	}
	value, found := p.fields[marker].Value(reflect.ValueOf(target))
	if !found {
		return false, true, nil
	}
	return value.Bool(), true, nil
}

// Only the approved whole-body conversion opts into root policy ownership.
// Other jsonx metadata keeps its existing owner and must not gain new restrictions.
func hasObjectToSlicePolicy(authored string) bool {
	for _, option := range strings.Split(authored, ",") {
		if strings.TrimSpace(option) == "objectToSlice" {
			return true
		}
	}
	return false
}
