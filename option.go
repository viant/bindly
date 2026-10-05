package bindly

import (
	"github.com/viant/bindly/locator"
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

func WithCache[T any](cache *ValueCache) BindingOption[T] {
	return func(b *BindingContext[T]) {
		b.valueCache = cache
	}
}

func WithAllowedKinds[T any](kinds ...string) BindingOption[T] {
	return func(c *BindingContext[T]) { c.allowedKinds = kindSet(kinds) }
}
func WithDelayedLocator[T any](kinds ...string) BindingOption[T] {
	return func(c *BindingContext[T]) { c.delayedKinds = kindSet(kinds) }
}
func WithMissingPolicy[T any](strict bool) BindingOption[T] {
	return func(c *BindingContext[T]) { c.strictMissing = strict }
}
