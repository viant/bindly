package bindly

import (
	"fmt"
	"strings"
)

func (p *Plan) Replay(paths ...string) (*ReplayPlan, error) {
	if p == nil {
		return nil, fmt.Errorf("replay requires a binding plan")
	}
	result := &ReplayPlan{plan: p, fields: map[string]BindingSpec{}}
	names := map[string]bool{}
	for _, path := range paths {
		var selected *BindingSpec
		for _, binding := range p.bindings {
			if binding.Path == path {
				copy := binding
				selected = &copy
				break
			}
		}
		if selected == nil {
			return nil, fmt.Errorf("replay binding %s is not defined", path)
		}
		name := selected.Name
		if name == "" {
			name = path
		}
		selected.Name = name
		if names[name] {
			return nil, fmt.Errorf("duplicate replay parameter %s", name)
		}
		names[name] = true

		result.fields[path] = *selected
		result.order = append(result.order, path)
	}
	result.preflight = result.fields
	return result, nil
}

// Prepare includes fresh, non-persisted bindings needed to authorize external
// input. Param dependencies are resolved through the plan's canonical aliases.
func (p *ReplayPlan) Prepare(projection *Projection, paths ...string) (*ReplayPlan, error) {
	if projection == nil || projection.TargetType() != p.plan.target {
		return nil, fmt.Errorf("replay preparation requires the canonical input projection")
	}
	result := &ReplayPlan{plan: p.plan, fields: p.fields, order: p.order, preflight: map[string]BindingSpec{}, protected: map[string]bool{}}
	for path, binding := range p.preflight {
		result.preflight[path] = binding
	}
	for path, protected := range p.protected {
		result.protected[path] = protected
	}
	active := map[string]bool{}
	var visit func(string) error
	visit = func(path string) error {
		result.protected[path] = true
		if _, ok := result.preflight[path]; ok {
			return nil
		}
		if active[path] {
			return fmt.Errorf("cyclic replay preparation %s", path)
		}
		active[path] = true
		defer delete(active, path)
		var binding *BindingSpec
		for _, candidate := range p.plan.bindings {
			if candidate.Path == path {
				copy := candidate
				binding = &copy
				break
			}
		}
		if binding == nil {
			return fmt.Errorf("replay preparation binding %s is absent", path)
		}
		switch binding.Location.Kind {
		case "param":
			source, ok := projection.paths[strings.ToLower(strings.TrimSpace(binding.Location.In))]
			if !ok {
				return fmt.Errorf("replay preparation source %s is absent", binding.Location.In)
			}
			if err := visit(source); err != nil {
				return err
			}
		case "const", "literal", "env":
		default:
			return fmt.Errorf("replay authorization requires an external or pure parameter source for %s", path)
		}
		result.preflight[path] = *binding
		return nil
	}
	for _, path := range paths {
		if err := visit(path); err != nil {
			return nil, err
		}
	}
	return result, nil
}
