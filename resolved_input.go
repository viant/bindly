package bindly

import (
	"fmt"
	"reflect"
	"strings"

	xshape "github.com/viant/x/shape"
)

// WithResolvedInput imports explicitly selected, already-transformed canonical
// destination values for this Bind target only. It is trusted internal input,
// not transport replay or provider-cache authority. Callers own capture of
// detachedInput and must not mutate it concurrently with Bind. Canonical,
// cacheable unscoped component pointers are borrowed by identity, read-only
// throughout the invocation; ordinary selected data remains detached.
// WithPlan must supply the identical plan; replay cannot be combined with this
// option. Normal conditions, validation, assignment and observation still run.
// Dependency-mode groups allow only paths disjoint from every group member and
// its presence marker. Those selected values bind before any group is opened.
func WithResolvedInput(plan *Plan, detachedInput any, paths ...string) BindOption {
	selection := append([]string(nil), paths...)
	return func(options *bindOptions) {
		if options.resolvedInput != nil {
			options.resolvedError = fmt.Errorf("resolved input option may only be supplied once")
			return
		}
		options.resolvedInput = &resolvedInput{plan: plan, input: detachedInput, paths: selection}
	}
}

type resolvedInput struct {
	plan  *Plan
	input any
	paths []string
}

func prepareResolvedInput(options *bindOptions, target any) (map[string]any, error) {
	if options.resolvedError != nil {
		return nil, options.resolvedError
	}
	seed := options.resolvedInput
	if seed == nil {
		return nil, nil
	}
	if seed.plan == nil || options.plan != seed.plan {
		return nil, fmt.Errorf("resolved input requires the identical canonical binding plan")
	}
	if options.replay != nil {
		return nil, fmt.Errorf("resolved input cannot be combined with replay")
	}
	expected := reflect.PointerTo(seed.plan.target)
	for _, input := range []any{target, seed.input} {
		value := reflect.ValueOf(input)
		if !value.IsValid() || value.Type() != expected || value.IsNil() {
			return nil, fmt.Errorf("resolved input and target must be non-nil %s", expected)
		}
	}
	if err := validateResolvedGroupPaths(seed.plan, seed.paths); err != nil {
		return nil, err
	}
	declared := make(map[string]BindingSpec, len(seed.plan.bindings))
	for _, binding := range seed.plan.bindings {
		declared[binding.Path] = binding
	}
	selection := xshape.CloneOptions{}
	seen := make(map[string]bool, len(seed.paths))
	var detachedPaths []string
	borrowed := map[string]any{}
	for _, path := range seed.paths {
		binding, defined := declared[path]
		if !defined {
			return nil, fmt.Errorf("resolved input binding %s is not defined", path)
		}
		if seen[path] {
			return nil, fmt.Errorf("duplicate resolved input binding %s", path)
		}
		seen[path] = true
		field, ok := seed.plan.fields[path].Value(reflect.ValueOf(seed.input))
		if !ok {
			return nil, fmt.Errorf("resolved input binding %s is inaccessible", path)
		}
		if strings.EqualFold(binding.Location.Kind, "component") && binding.Scope == "" && (binding.Cacheable == nil || *binding.Cacheable) && field.Kind() == reflect.Pointer {
			borrowed[path] = field.Interface()
		} else {
			detachedPaths = append(detachedPaths, path)
		}
	}
	if err := selection.Select(seed.plan.target, detachedPaths...); err != nil {
		return nil, err
	}
	cloned, err := (xshape.Runtime{}).CloneValue(seed.input, selection)
	if err != nil {
		return nil, fmt.Errorf("clone resolved input: %w", err)
	}
	value := reflect.ValueOf(cloned)
	result := make(map[string]any, len(seed.paths))
	for _, path := range seed.paths {
		if ref, ok := borrowed[path]; ok {
			result[path] = ref
			continue
		}
		field, ok := seed.plan.fields[path].Value(value)
		if !ok {
			return nil, fmt.Errorf("resolved input binding %s is inaccessible", path)
		}
		result[path] = field.Interface()
	}
	return result, nil
}

func validateResolvedGroupPaths(plan *Plan, paths []string) error {
	if len(plan.groups) == 0 {
		return nil
	}
	for _, group := range plan.groups {
		if group.mode != ResolutionGroupDependencies {
			return fmt.Errorf("resolved input requires dependency-mode resolution groups")
		}
	}
	for _, path := range paths {
		selected, exists := plan.fields[path]
		if !exists {
			continue // The ordinary resolved-input validation reports unknown paths.
		}
		writes := [][]int{selected.Index}
		for _, binding := range plan.bindings {
			if binding.Path == path && binding.MarkerField != "" {
				writes = append(writes, plan.fields[binding.MarkerField].Index)
				break
			}
		}
		for _, group := range plan.groups {
			for _, member := range group.members {
				for _, protected := range []string{member.Path, member.MarkerField} {
					for _, write := range writes {
						if protected != "" && overlappingFieldIndexes(write, plan.fields[protected].Index) {
							return fmt.Errorf("resolved input binding %s overlaps resolution group binding %s", path, member.Path)
						}
					}
				}
			}
		}
	}
	return nil
}

func overlappingFieldIndexes(left, right []int) bool {
	for i := 0; i < len(left) && i < len(right); i++ {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
