package bindly

import (
	"reflect"

	"github.com/viant/bindly/locator"
	"github.com/viant/bindly/state"
	"github.com/viant/bindly/xform"
	"github.com/viant/structology"
	"github.com/viant/tagly/tags"
)

// Binding represents a single field binding, including metadata parsed from struct tags,
// resource reference, and runtime resolver and transformer settings.
type Binding struct {
	selector  *structology.Selector
	path      string
	index     []int
	fieldType reflect.Type
	// sourceType is the provider-facing value contract. Transformation and
	// destination conversion happen only after provider lookup.
	sourceType reflect.Type
	// location holds kind and in-path information
	location *state.Location
	// Metadata fields (from bind tag)
	Name         string
	Scope        string
	When         string
	ErrorCode    int
	ErrorMessage string
	DataType     string
	Cardinality  string
	With         string
	URI          string
	ResourceRef  string
	Required     *bool
	Cacheable    *bool
	Async        bool
	Tags         tags.Values
	DefaultValue interface{}
	Tag          reflect.StructTag
	// Extension holds optional immutable metadata supplied by a plan compiler.
	Extension interface{}
	// Runtime fields set during plan compilation.
	priority    int
	transformer xform.Transformer
	XformConfig tags.Values
}

func (b *Binding) destinationPath() string {
	if b.path != "" {
		return b.path
	}
	if b.selector != nil {
		return b.selector.Path()
	}
	return ""
}

func (b *Binding) destinationType() reflect.Type {
	if b.fieldType != nil {
		return b.fieldType
	}
	if b.selector != nil {
		return b.selector.Type()
	}
	return nil
}

func (b *Binding) providerType() reflect.Type {
	if b.sourceType != nil {
		return b.sourceType
	}
	return b.destinationType()
}

// Kind returns binding location kind.
func (b *Binding) Kind() string {
	if b == nil || b.location == nil {
		return ""
	}
	return b.location.Kind
}

// In returns binding location in-path.
func (b *Binding) In() string {
	if b == nil || b.location == nil {
		return ""
	}
	return b.location.In
}

// NameTag returns logical parameter name.
func (b *Binding) NameTag() string {
	return b.Name
}

// ScopeTag returns binding scope.
func (b *Binding) ScopeTag() string {
	return b.Scope
}

// IsRequired reports whether binding is required.
func (b *Binding) IsRequired() bool {
	return b != nil && b.Required != nil && *b.Required
}

// IsCacheable reports whether binding result should be cached.
func (b *Binding) IsCacheable() bool {
	return b != nil && b.Cacheable != nil && *b.Cacheable
}

func (b *Binding) cacheEnabled(provider locator.Provider) bool {
	if b.Cacheable != nil {
		return *b.Cacheable
	}
	policy, ok := provider.(locator.CachePolicy)
	return ok && policy.DefaultCacheable()
}

func (b *Binding) cacheKey() string {
	if b.Name != "" {
		return b.Name
	}
	return b.destinationPath()
}

// Default returns default value associated with this binding.
func (b *Binding) Default() interface{} {
	return b.DefaultValue
}

// Location returns underlying state.Location.
func (b *Binding) Location() *state.Location {
	return b.location
}
