package bindly

import (
	"github.com/viant/bindly/locator"
	"github.com/viant/bindly/resource"
	"github.com/viant/bindly/xform"
	"github.com/viant/bindly/xform/conv"
)

// Injector represents dependency injector
type Injector struct {
	parent          *Injector
	scopedProviders []locator.Provider
	resources       *resource.Store
	initErr         error
	locators        *locator.Registry
	transformers    *xform.Registry
	providers       []locator.Provider
	bindingTag      string
	xformTag        string
	interfaceKind   string
	bindingCache    *BindingCache
	structTypeCache *StructTypeCache
	valueCache      *ValueCache
}

// NewInjector creates injector
func NewInjector(options ...InjectorOption) (*Injector, error) {
	ret := &Injector{
		resources:       resource.New(),
		valueCache:      NewValueCache(),
		locators:        locator.NewRegistry(),
		transformers:    xform.NewRegistry(),
		bindingTag:      bindingTag,
		xformTag:        xFormTag,
		interfaceKind:   "interface",
		bindingCache:    NewBindingCache(),
		structTypeCache: NewStructTypeCache(),
	}

	for _, option := range options {
		option(ret)
	}
	if ret.transformers != nil {
		conv.Init(ret.transformers)
	}
	if len(ret.providers) > 0 {
		ret.scopedProviders = append([]locator.Provider(nil), ret.providers...)
		for _, provider := range ret.providers {
			if err := ret.locators.Register(provider); err != nil {
				return nil, err
			}
		}
		ret.providers = nil
	}
	if ret.initErr != nil {
		return nil, ret.initErr
	}
	return ret, nil
}

// TransformerRegistry returns the transformer registry
func (b *Injector) TransformerRegistry() *xform.Registry {
	return b.transformers
}

// HasProvider reports whether a provider kind is registered in this injector's
// lookup ancestry. It does not resolve values or confer binding authority.
func (b *Injector) HasProvider(kind string) bool {
	for current := b; current != nil; current = current.parent {
		if _, ok := current.locators.Lookup(kind); ok {
			return true
		}
	}
	return false
}
