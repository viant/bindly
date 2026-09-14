package bindly

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/viant/bindly/locator"
)

// CaptureSources snapshots only this plan's selected source bindings. Providers
// are invocation-scoped; no target is initialized, transformer applied, default
// materialized or prerequisite/dependency evaluated here. Raw-source hooks retain
// JSON presence and exact numbers before the normal ReplayBinding preflight.
func (p *ReplayPlan) CaptureSources(ctx context.Context, providers []locator.Provider) (*Replay, error) {
	if p == nil || p.plan == nil || ctx == nil {
		return nil, fmt.Errorf("replay source capture requires a plan and context")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	for _, provider := range providers {
		if nilValue(provider) {
			return nil, fmt.Errorf("replay source provider is required")
		}
	}
	injector, err := NewInjector(WithProviders(providers...))
	if err != nil {
		return nil, err
	}
	scope := &invocation{injector: injector, active: map[string]bool{}, cache: map[resolutionKey]resolution{}, captureSources: true}
	result := &Replay{plan: p, raw: map[string]json.RawMessage{}}
	for _, path := range p.order {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		binding := p.fields[path]
		target := binding.SourceType
		if target == nil {
			if binding.Transformer != nil {
				return nil, fmt.Errorf("replay codec %s requires a declared source type", binding.Name)
			}
			target = p.plan.fields[path].Type
		}
		value, err := scope.resolveResult(ctx, &binding.Location, target, binding.Cacheable)
		if err != nil {
			return nil, &BindingError{Path: path, Code: binding.ErrorCode, Message: binding.ErrorMessage, Cause: err}
		}
		if !value.found {
			continue
		}
		if _, raw := value.value.(json.RawMessage); !raw {
			if err = value.convert(target); err != nil {
				return nil, &BindingError{Path: path, Code: binding.ErrorCode, Message: binding.ErrorMessage, Cause: err}
			}
		}
		encoded, err := json.Marshal(value.value)
		if err != nil {
			return nil, &BindingError{Path: path, Code: binding.ErrorCode, Message: binding.ErrorMessage, Cause: err}
		}
		result.raw[binding.Name] = encoded
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return result, nil
}
