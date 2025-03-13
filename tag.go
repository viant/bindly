package bindly

import (
	"context"
	"embed"
	"fmt"
	"github.com/viant/tagly/tags"
)

const bindingTag = "bind"

// extractBinding extracts binding from struct tag
func (b *Injector) extractBinding(aBinding *Binding) {

	tag, ok := aBinding.selector.Tag().Lookup(b.bindingTag)
	if !ok {
		return
	}
	tagValue := tags.Values(tag)
	_ = tagValue.MatchPairs(func(key, value string) error {
		switch key {
		case "in":
			aBinding.location.In = value
		case "kind":
			aBinding.location.Kind = value
		case "cacheable":
			aBinding.cachable = true
		}
		return nil
	})

	if aBinding.location.Kind == "" && aBinding.location.In != "" {
		aBinding.location.Kind = "state"
	}

}

const xFormTag = "xform"

// extractTransformer extracts transformer from struct tag
func (b *Injector) extractTransformer(ctx context.Context, aBinding *Binding, embedFs *embed.FS) error {
	// Check if field has a transformer configured
	name := ""
	tag, ok := aBinding.selector.Tag().Lookup(b.xformTag)
	if !ok {
		return nil
	}
	tagValue := tags.Values(tag)
	name, tagValue = tagValue.Name()
	factory, ok := b.transformers.Lookup(name)
	if !ok {
		return fmt.Errorf("failed to lookup transformer: %v", name)
	}
	transformer, err := factory.Create(ctx, tagValue, aBinding.selector.Type(), embedFs)
	if err != nil {
		return fmt.Errorf("failed to create transformer: %v, %w", name, err)
	}
	aBinding.transformer = transformer
	return nil
}
