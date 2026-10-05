package bindly

import (
	"context"
	"embed"
	"fmt"
	"github.com/viant/tagly/tags"
	"reflect"
)

const bindingTag = "bind"
const xFormTag = "xform"

func (i *Injector) extractBinding(binding *Binding) {
	spec, found, err := bindingSpecFromField(reflect.StructField{Name: binding.selector.Path(), Tag: binding.selector.Tag()}, i.bindingTag, i.bindingTag == bindingTag)
	if err != nil || !found {
		return
	}
	binding.location = &spec.Location
	binding.Name = spec.Name
	binding.Required = spec.Required
	binding.Cacheable = spec.Cacheable
	binding.DefaultValue = spec.DefaultValue
}
func (i *Injector) extractTransformer(ctx context.Context, binding *Binding, embedFS *embed.FS) error {
	raw, found := binding.selector.Tag().Lookup(i.xformTag)
	if !found {
		return nil
	}
	name, args := tags.Values(raw).Name()
	factory, found := i.transformers.Lookup(name)
	if !found {
		return fmt.Errorf("failed to lookup transformer: %s", name)
	}
	transformer, err := factory.Create(ctx, args, binding.selector.Type(), i.resources)
	if err != nil {
		return err
	}
	binding.transformer = transformer
	return nil
}
