package bindly

import (
	"fmt"
	"reflect"

	xshape "github.com/viant/x/shape"
)

// WithResolvedInput imports explicitly selected, already-transformed canonical
// destination values for this Bind target only. It is trusted internal input,
// not transport replay or provider-cache authority. Callers own capture of
// detachedInput and must not mutate it concurrently with Bind.
// WithPlan must supply the identical plan; replay cannot be combined with this
// option. Normal conditions, validation, assignment and observation still run.
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
	declared := make(map[string]bool, len(seed.plan.bindings))
	for _, binding := range seed.plan.bindings {
		declared[binding.Path] = true
	}
	selection := xshape.CloneOptions{}
	seen := make(map[string]bool, len(seed.paths))
	for _, path := range seed.paths {
		if !declared[path] {
			return nil, fmt.Errorf("resolved input binding %s is not defined", path)
		}
		if seen[path] {
			return nil, fmt.Errorf("duplicate resolved input binding %s", path)
		}
		seen[path] = true
	}
	if err := selection.Select(seed.plan.target, seed.paths...); err != nil {
		return nil, err
	}
	cloned, err := (xshape.Runtime{}).CloneValue(seed.input, selection)
	if err != nil {
		return nil, fmt.Errorf("clone resolved input: %w", err)
	}
	value := reflect.ValueOf(cloned)
	result := make(map[string]any, len(seed.paths))
	for _, path := range seed.paths {
		field, ok := seed.plan.fields[path].Value(value)
		if !ok {
			return nil, fmt.Errorf("resolved input binding %s is inaccessible", path)
		}
		result[path] = field.Interface()
	}
	return result, nil
}
