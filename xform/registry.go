package xform

import "github.com/viant/bindly/internal"

type Registry struct {
	internal.Map[string, Factory]
}

// Register adds a transformer to the registry
func (r *Registry) Register(name string, factory Factory) {
	r.Put(name, factory)
}

// Lookup returns a transformer from the registry
func (r *Registry) Lookup(name string) (Factory, bool) {
	return r.Get(name)
}

func NewRegistry() *Registry {
	return &Registry{
		Map: internal.NewMap[string, Factory](),
	}
}
