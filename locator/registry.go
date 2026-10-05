package locator

import (
	"fmt"
	"sort"

	"github.com/viant/bindly/internal"
)

type Registry struct {
	internal.Map[string, Provider]
	parent *Registry
}

// Register registers a provider for a given kind
func (r *Registry) Register(provider Provider) error {
	if r.Exists(provider.Kind()) {
		return fmt.Errorf("kind: %v is already registered", provider.Kind())
	}
	r.Put(provider.Kind(), provider)
	return nil
}

// Unregister unregisters a provider for a given kind
func (r *Registry) Unregister(kind string) {
	r.Delete(kind)
}

// Lookup returns a provider for a given kind
func (r *Registry) Lookup(kind string) (Provider, bool) {
	if r == nil {
		return nil, false
	}
	if provider, ok := r.Get(kind); ok {
		return provider, true
	}
	return r.parent.Lookup(kind)
}

// Keys returns the distinct provider kinds visible in this registry. Local
// providers shadow parent providers with the same kind.
func (r *Registry) Keys() []string {
	if r == nil {
		return nil
	}
	kinds := map[string]struct{}{}
	if r.parent != nil {
		for _, kind := range r.parent.Keys() {
			kinds[kind] = struct{}{}
		}
	}
	for _, kind := range r.Map.Keys() {
		kinds[kind] = struct{}{}
	}
	result := make([]string, 0, len(kinds))
	for kind := range kinds {
		result = append(result, kind)
	}
	sort.Strings(result)
	return result
}

// Child creates an empty provider layer that falls back to this registry.
func (r *Registry) Child() *Registry {
	return &Registry{Map: internal.NewMap[string, Provider](), parent: r}
}

func NewRegistry() *Registry {
	return &Registry{Map: internal.NewMap[string, Provider]()}
}
