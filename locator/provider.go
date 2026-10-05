package locator

import "github.com/viant/structology"

type Provider interface {
	Locate(state *structology.State) Locator
	Kind() string
	Priority() int
}

// CachePolicy declares the provider's default when a binding does not specify
// cacheable explicitly. A binding-level option always wins.
type CachePolicy interface {
	DefaultCacheable() bool
}
