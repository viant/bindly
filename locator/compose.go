package locator

import (
	"context"
	"fmt"
	"reflect"

	"github.com/viant/structology"
)

type ProviderLayer struct {
	Name     string
	Provider Provider
}
type composedProvider struct {
	kind   string
	layers []ProviderLayer
}
type composedLocator struct {
	provider *composedProvider
	state    *structology.State
}

func ComposeProviders(kind string, layers ...ProviderLayer) (Provider, error) {
	if kind == "" || len(layers) == 0 {
		return nil, fmt.Errorf("provider composition requires kind and layers")
	}
	seen := map[string]bool{}
	for _, layer := range layers {
		if layer.Name == "" || seen[layer.Name] {
			return nil, fmt.Errorf("provider layer name %q must be nonempty and unique", layer.Name)
		}
		seen[layer.Name] = true
		if layer.Provider == nil || layer.Provider.Kind() != kind {
			return nil, fmt.Errorf("provider layer %s must supply kind %s", layer.Name, kind)
		}
	}
	return &composedProvider{kind: kind, layers: append([]ProviderLayer(nil), layers...)}, nil
}
func (p *composedProvider) Kind() string { return p.kind }
func (p *composedProvider) Priority() int {
	priority := 0
	for _, layer := range p.layers {
		if candidate := layer.Provider.Priority(); candidate > priority {
			priority = candidate
		}
	}
	return priority
}
func (p *composedProvider) DefaultCacheable() bool {
	for _, layer := range p.layers {
		policy, ok := layer.Provider.(CachePolicy)
		if !ok || !policy.DefaultCacheable() {
			return false
		}
	}
	return true
}
func (p *composedProvider) Locate(state *structology.State) Locator {
	return &composedLocator{provider: p, state: state}
}
func (l *composedLocator) Kind() string { return l.provider.kind }
func (l *composedLocator) Value(ctx context.Context, target reflect.Type, name string) (any, bool, error) {
	return l.ValueInScope(ctx, nil, target, name)
}
func (l *composedLocator) ValueInScope(ctx context.Context, scope Scope, target reflect.Type, name string) (any, bool, error) {
	return l.value(ctx, scope, target, name, false, "")
}
func (l *composedLocator) CaptureSource(ctx context.Context, target reflect.Type, name string) (any, bool, error) {
	return l.value(ctx, nil, target, name, true, "")
}
func (l *composedLocator) ValueWithBodyNullPolicy(ctx context.Context, target reflect.Type, name, policy string) (any, bool, error) {
	return l.value(ctx, nil, target, name, false, policy)
}
func (l *composedLocator) value(ctx context.Context, scope Scope, target reflect.Type, name string, capture bool, policy string) (any, bool, error) {
	for _, layer := range l.provider.layers {
		candidate := layer.Provider.Locate(l.state)
		if candidate == nil {
			return nil, false, fmt.Errorf("provider layer %s returned no locator", layer.Name)
		}
		var value any
		var found bool
		var err error
		var raw SourceCapturer
		if capture {
			raw, _ = candidate.(SourceCapturer)
		}
		if raw != nil {
			value, found, err = raw.CaptureSource(ctx, target, name)
		} else if policy != "" {
			policyLocator, ok := candidate.(BodyNullPolicyLocator)
			if !ok {
				return nil, false, fmt.Errorf("provider layer %s does not support bodyNullPolicy", layer.Name)
			}
			value, found, err = policyLocator.ValueWithBodyNullPolicy(ctx, target, name, policy)
		} else if scoped, ok := candidate.(ScopedLocator); ok && scope != nil {
			value, found, err = scoped.ValueInScope(ctx, scope, target, name)
		} else {
			value, found, err = candidate.Value(ctx, target, name)
		}
		owned, _ := candidate.(AuthoritativeLocator)
		if found || err != nil || (owned != nil && owned.Owns(name)) {
			return value, found, err
		}
	}
	return nil, false, nil
}

func (l *composedLocator) Owns(name string) bool {
	for _, layer := range l.provider.layers {
		candidate := layer.Provider.Locate(l.state)
		if owned, ok := candidate.(AuthoritativeLocator); ok && owned.Owns(name) {
			return true
		}
	}
	return false
}
