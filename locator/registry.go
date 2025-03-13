package locator

import (
	"fmt"
	"github.com/viant/bindly/internal"
)

type Registry struct {
	internal.Map[string, Provider]
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
	return r.Get(kind)
}

func NewRegistry() *Registry {
	return &Registry{internal.NewMap[string, Provider]()}
}
