package bindly

import (
	"fmt"
	"io/fs"

	"github.com/viant/bindly/locator"
	"github.com/viant/bindly/resource"
	"github.com/viant/bindly/xform"
	"github.com/viant/bindly/xform/conv"
)

// Injector represents dependency injector
type Injector struct {
	locators        *locator.Registry
	transformers    *xform.Registry
	providers       []locator.Provider
	bindingTag      string
	xformTag        string
	interfaceKind   string
	bindingCache    *BindingCache
	structTypeCache *StructTypeCache
	valueCache      *ValueCache
	resources       *resource.Store
	optionErr       error
}

// NewInjector creates an injector and validates its provider registry.
func NewInjector(options ...InjectorOption) (*Injector, error) {
	ret := &Injector{
		locators:        locator.NewRegistry(),
		transformers:    xform.NewRegistry(),
		bindingTag:      bindingTag,
		xformTag:        xFormTag,
		interfaceKind:   "interface",
		bindingCache:    NewBindingCache(),
		structTypeCache: NewStructTypeCache(),
		valueCache:      NewValueCache(),
		resources:       resource.New(),
	}

	for _, option := range options {
		option(ret)
	}
	if ret.optionErr != nil {
		return nil, ret.optionErr
	}

	if ret.transformers != nil {
		conv.Init(ret.transformers)
	}
	if len(ret.providers) > 0 {
		if err := registerProviders(ret.locators, ret.providers); err != nil {
			return nil, err
		}
		ret.providers = nil
	}
	return ret, nil
}

// ForScope creates an invocation child whose local providers shadow the
// application injector while caches and immutable transformer registries are
// shared. Duplicate kinds within the same scope are rejected.
func (i *Injector) ForScope(providers ...locator.Provider) (*Injector, error) {
	if i == nil {
		return nil, fmt.Errorf("parent injector is required")
	}
	child := &Injector{
		locators:        i.locators.Child(),
		transformers:    i.transformers,
		bindingTag:      i.bindingTag,
		xformTag:        i.xformTag,
		interfaceKind:   i.interfaceKind,
		bindingCache:    i.bindingCache,
		structTypeCache: i.structTypeCache,
		valueCache:      NewValueCache(),
		resources:       i.resources,
	}
	if err := registerProviders(child.locators, providers); err != nil {
		return nil, err
	}
	return child, nil
}

func registerProviders(registry *locator.Registry, providers []locator.Provider) error {
	for _, provider := range providers {
		if provider == nil {
			return fmt.Errorf("locator provider is required")
		}
		if err := registry.Register(provider); err != nil {
			return err
		}
	}
	return nil
}

// TransformerRegistry returns the transformer registry
func (b *Injector) TransformerRegistry() *xform.Registry {
	return b.transformers
}

// ReadResource reads a namespaced resource registered with the injector.
func (b *Injector) ReadResource(reference string) ([]byte, error) {
	if b == nil || b.resources == nil {
		return nil, fmt.Errorf("injector resource store is not configured")
	}
	return b.resources.ReadFile(reference)
}

// Resources returns the injector's shared resource store. Scoped injectors
// retain this exact store so compilers and runtime providers see one source of
// authority for package resources.
func (b *Injector) Resources() *resource.Store {
	if b == nil {
		return nil
	}
	return b.resources
}

// ResourceFS returns the original filesystem registered for a namespace.
func (b *Injector) ResourceFS(name string) (fs.FS, bool) {
	if b == nil || b.resources == nil {
		return nil, false
	}
	return b.resources.Filesystem(name)
}
