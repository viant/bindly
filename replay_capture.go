package bindly

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/viant/bindly/locator"
	xshape "github.com/viant/x/shape"
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
	return p.CaptureSourcesIn(ctx, injector)
}

// CaptureSourcesIn snapshots raw sources using an already composed invocation
// injector. It uses no persistent value cache and never binds a target, applies
// a transformer, or evaluates a prerequisite. The caller owns scope lifetime.
func (p *ReplayPlan) CaptureSourcesIn(ctx context.Context, injector *Injector) (*Replay, error) {
	if p == nil || p.plan == nil || ctx == nil || injector == nil || injector.locators == nil {
		return nil, fmt.Errorf("replay source capture requires a plan, context and injector")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	scope := &invocation{injector: injector, active: map[string]bool{}, cache: map[resolutionKey]resolution{}, captureSources: true}
	result := &Replay{plan: p, raw: map[string]json.RawMessage{}, sourceValues: map[string]any{}, sourceEvidence: true}
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
		value, err := scope.resolveBindingResult(ctx, &binding.Location, target, binding.Cacheable, "", binding.PresenceOnly, binding.BodyRepresentation, binding.bodyRepresentationKey)
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
		if detached, cloneErr := (xshape.Runtime{}).CloneValue(value.value); cloneErr == nil {
			result.sourceValues[binding.Name] = detached
		} else {
			result.sourceEvidence = false
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
