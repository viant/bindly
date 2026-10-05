package bindly

import (
	"io/fs"

	"github.com/viant/bindly/locator"
	"github.com/viant/bindly/resource"
)

type InjectorOption func(*Injector)
type BindingOption[T any] func(ctx *BindingContext[T])

func WithLocators(registry *locator.Registry) InjectorOption {
	return func(b *Injector) {
		b.locators = registry
	}
}
func WithProviders(providers ...locator.Provider) InjectorOption {
	return func(b *Injector) {
		b.providers = append(b.providers, providers...)
	}
}

func WithBindingTag(tag string) InjectorOption {
	return func(b *Injector) {
		b.bindingTag = tag
	}
}

func WithTransformerTag(tag string) InjectorOption {
	return func(b *Injector) {
		b.xformTag = tag
	}
}

func WithResources(resources *resource.Store) InjectorOption {
	return func(injector *Injector) {
		if resources != nil {
			injector.resources = resources
		}
	}
}

// WithResourceFS registers a standard filesystem namespace. A go:embed value
// can be passed directly because embed.FS implements fs.FS.
func WithResourceFS(name string, source fs.FS) InjectorOption {
	return func(injector *Injector) {
		if injector.optionErr != nil {
			return
		}
		injector.optionErr = injector.resources.Register(name, source)
	}
}

func WithCache[T any](cache *ValueCache) BindingOption[T] {
	return func(b *BindingContext[T]) {
		b.valueCache = cache
	}
}

// WithAllowedKinds restricts binding resolution to the specified kinds.
func WithAllowedKinds[T any](kinds ...string) BindingOption[T] {
	set := make(map[string]bool, len(kinds))
	for _, k := range kinds {
		set[k] = true
	}
	return func(ctx *BindingContext[T]) {
		ctx.allowedKinds = set
	}
}

// WithDelayedLocator skips resolution for the specified kinds in this bind pass.
func WithDelayedLocator[T any](kinds ...string) BindingOption[T] {
	set := make(map[string]bool, len(kinds))
	for _, k := range kinds {
		set[k] = true
	}
	return func(ctx *BindingContext[T]) {
		ctx.delayedKinds = set
	}
}

// WithMissingPolicy controls strict (true) or lenient (false) handling of missing Required parameters.
func WithMissingPolicy[T any](strict bool) BindingOption[T] {
	return func(ctx *BindingContext[T]) {
		ctx.strictMissing = strict
	}
}
