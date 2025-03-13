package bindly

import (
	"github.com/viant/bindly/locator"
	"github.com/viant/bindly/types"
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
	embedder        types.Embedder
}

// NewInjector creates injector
func NewInjector(options ...InjectorOption) *Injector {
	ret := &Injector{
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
		for _, provider := range ret.providers {
			_ = ret.locators.Register(provider)
		}
		ret.providers = nil
	}
	return ret
}

// TransformerRegistry returns the transformer registry
func (b *Injector) TransformerRegistry() *xform.Registry {
	return b.transformers
}
