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
	for _, layer := range layers {
		if layer.Provider == nil || layer.Provider.Kind() != kind {
			return nil, fmt.Errorf("provider layer %s must supply kind %s", layer.Name, kind)
		}
	}
	return &composedProvider{kind: kind, layers: append([]ProviderLayer(nil), layers...)}, nil
}
func (p *composedProvider) Kind() string  { return p.kind }
func (p *composedProvider) Priority() int { return p.layers[0].Provider.Priority() }
func (p *composedProvider) DefaultCacheable() bool {
	policy, ok := p.layers[0].Provider.(CachePolicy)
	return ok && policy.DefaultCacheable()
}
func (p *composedProvider) Locate(state *structology.State) Locator {
	return &composedLocator{provider: p, state: state}
}
func (l *composedLocator) Kind() string { return l.provider.kind }
func (l *composedLocator) Value(ctx context.Context, target reflect.Type, name string) (any, bool, error) {
	return l.ValueInScope(ctx, nil, target, name)
}
func (l *composedLocator) ValueInScope(ctx context.Context, scope Scope, target reflect.Type, name string) (any, bool, error) {
	for _, layer := range l.provider.layers {
		candidate := layer.Provider.Locate(l.state)
		if candidate == nil {
			return nil, false, fmt.Errorf("provider layer %s returned no locator", layer.Name)
		}
		var value any
		var found bool
		var err error
		if scoped, ok := candidate.(ScopedLocator); ok && scope != nil {
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
