package locator

import (
	"context"
	"fmt"
	"reflect"

	"github.com/viant/structology"
)

// ProviderLayer names one authority level in a same-kind provider chain.
type ProviderLayer struct {
	Name     string
	Provider Provider
}

// ComposeProviders creates one provider that resolves the first value found in
// highest-to-lowest authority order.
func ComposeProviders(kind string, layers ...ProviderLayer) (Provider, error) {
	if kind == "" {
		return nil, fmt.Errorf("provider kind is required")
	}
	if len(layers) == 0 {
		return nil, fmt.Errorf("provider kind %q requires at least one layer", kind)
	}
	seen := make(map[string]struct{}, len(layers))
	result := &compositeProvider{kind: kind, layers: make([]ProviderLayer, len(layers))}
	for index, layer := range layers {
		if layer.Name == "" {
			return nil, fmt.Errorf("provider kind %q layer %d name is required", kind, index)
		}
		if _, ok := seen[layer.Name]; ok {
			return nil, fmt.Errorf("provider kind %q layer name %q is duplicated", kind, layer.Name)
		}
		seen[layer.Name] = struct{}{}
		if layer.Provider == nil {
			return nil, fmt.Errorf("provider kind %q layer %q is required", kind, layer.Name)
		}
		if actual := layer.Provider.Kind(); actual != kind {
			return nil, fmt.Errorf("provider layer %q kind %q does not match %q", layer.Name, actual, kind)
		}
		result.layers[index] = layer
		if priority := layer.Provider.Priority(); priority > result.priority {
			result.priority = priority
		}
		policy, ok := layer.Provider.(CachePolicy)
		if index == 0 {
			result.cacheable = ok && policy.DefaultCacheable()
		} else {
			result.cacheable = result.cacheable && ok && policy.DefaultCacheable()
		}
	}
	return result, nil
}

type compositeProvider struct {
	kind      string
	layers    []ProviderLayer
	priority  int
	cacheable bool
}

func (p *compositeProvider) Kind() string           { return p.kind }
func (p *compositeProvider) Priority() int          { return p.priority }
func (p *compositeProvider) DefaultCacheable() bool { return p.cacheable }

func (p *compositeProvider) Locate(state *structology.State) Locator {
	result := &compositeLocator{kind: p.kind, locators: make([]namedLocator, 0, len(p.layers))}
	for _, layer := range p.layers {
		candidate := layer.Provider.Locate(state)
		if candidate == nil {
			result.err = fmt.Errorf("provider kind %q layer %q returned no locator", p.kind, layer.Name)
			break
		}
		if actual := candidate.Kind(); actual != p.kind {
			result.err = fmt.Errorf("provider kind %q layer %q returned locator kind %q", p.kind, layer.Name, actual)
			break
		}
		result.locators = append(result.locators, namedLocator{name: layer.Name, locator: candidate})
	}
	return result
}

type namedLocator struct {
	name    string
	locator Locator
}

type compositeLocator struct {
	kind     string
	locators []namedLocator
	err      error
}

func (l *compositeLocator) Kind() string { return l.kind }

func (l *compositeLocator) Value(ctx context.Context, targetType reflect.Type, name string) (interface{}, bool, error) {
	return l.resolve(ctx, nil, targetType, name)
}

func (l *compositeLocator) ValueInScope(ctx context.Context, scope Scope, targetType reflect.Type, name string) (interface{}, bool, error) {
	return l.resolve(ctx, scope, targetType, name)
}

func (l *compositeLocator) resolve(ctx context.Context, scope Scope, targetType reflect.Type, name string) (interface{}, bool, error) {
	if l.err != nil {
		return nil, false, l.err
	}
	for _, candidate := range l.locators {
		var value interface{}
		var found bool
		var err error
		if scoped, ok := candidate.locator.(ScopedLocator); ok && scope != nil {
			value, found, err = scoped.ValueInScope(ctx, scope, targetType, name)
		} else {
			value, found, err = candidate.locator.Value(ctx, targetType, name)
		}
		if err != nil {
			return nil, false, fmt.Errorf("provider layer %q: %w", candidate.name, err)
		}
		if found {
			return value, true, nil
		}
	}
	return nil, false, nil
}
