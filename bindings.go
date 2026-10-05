package bindly

import (
	"context"
	"fmt"
	"github.com/viant/bindly/locator"
	"github.com/viant/structology"
	"reflect"
	"sort"
)

type Bindings []*Binding

// BindingType retains the public metadata surface; execution uses its canonical Plan.
type BindingType struct {
	Bindings []Bindings
	Type     *structology.StateType
	plan     *Plan
}

func (i *Injector) cachedPlan(target reflect.Type) (*Plan, error) {
	for target.Kind() == reflect.Pointer {
		target = target.Elem()
	}
	if cached, ok := i.bindingCache.Get(target); ok && cached.plan != nil {
		return cached.plan, nil
	}
	plan, err := i.CompilePlan(target)
	if err != nil {
		return nil, err
	}
	compatibility := i.bindingTypeFromPlan(plan)
	cached, _ := i.bindingCache.LoadOrStore(target, compatibility)
	return cached.plan, nil
}
func (i *Injector) bindingTypeFromPlan(plan *Plan) *BindingType {
	result := &BindingType{Type: structology.NewStateType(plan.target), plan: plan}
	var bindings Bindings
	for _, spec := range plan.bindings {
		location := spec.Location
		bindings = append(bindings, &Binding{path: spec.Path, index: append([]int(nil), plan.fields[spec.Path].Index...), fieldType: plan.fields[spec.Path].Type, sourceType: spec.SourceType, location: &location, Name: spec.Name, Scope: spec.Scope, When: spec.When, ErrorCode: spec.ErrorCode, ErrorMessage: spec.ErrorMessage, DataType: spec.DataType, Cardinality: spec.Cardinality, With: spec.With, URI: spec.URI, ResourceRef: spec.ResourceRef, Required: spec.Required, Cacheable: spec.Cacheable, Async: spec.Async, DefaultValue: spec.DefaultValue, Extension: spec.Extension, transformer: spec.Transformer})
	}
	if len(bindings) > 0 {
		result.Bindings = []Bindings{bindings}
	}
	return result
}
func (i *Injector) buildBindings(ctx context.Context, target *structology.StateType) (*BindingType, error) {
	plan, err := i.CompilePlan(target.Type())
	if err != nil {
		return nil, err
	}
	return i.bindingTypeFromPlan(plan), nil
}

// GroupByPriority orders legacy binding metadata using the active registry.
func (b Bindings) GroupByPriority(registry *locator.Registry) ([]Bindings, error) {
	priorities := map[*Binding]int{}
	for _, binding := range b {
		provider, found := registry.Lookup(binding.Kind())
		if !found {
			return nil, fmt.Errorf("failed to lookup binding provider for: %s, path: %s", binding.Kind(), binding.destinationPath())
		}
		priorities[binding] = provider.Priority()
	}
	sort.SliceStable(b, func(i, j int) bool { return priorities[b[i]] < priorities[b[j]] })
	var result []Bindings
	for _, binding := range b {
		if len(result) == 0 || priorities[result[len(result)-1][0]] != priorities[binding] {
			result = append(result, Bindings{binding})
		} else {
			result[len(result)-1] = append(result[len(result)-1], binding)
		}
	}
	return result, nil
}
