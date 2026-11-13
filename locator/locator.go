package locator

import (
	"context"
	"github.com/viant/x/shape"
	"reflect"
)

type Locator interface {
	Value(ctx context.Context, targetType reflect.Type, name string) (interface{}, bool, error)
	Kind() string
}

type Scope interface {
	Resolver
	Bind(context.Context, any) error
	BindTarget(context.Context, any) error
}

type ScopedLocator interface {
	Locator
	ValueInScope(context.Context, Scope, reflect.Type, string) (any, bool, error)
}

// AuthoritativeLocator distinguishes an owned absence from provider fallback.
type AuthoritativeLocator interface{ Owns(string) bool }

// SourceCapturer lets a source preserve its raw representation before target
// decoding (for example JSON body omission/number precision). JSON source bytes
// use json.RawMessage; other values retain normal declared source conversion.
// Implementations must not bind targets or evaluate downstream dependencies.
type SourceCapturer interface {
	CaptureSource(context.Context, reflect.Type, string) (any, bool, error)
}

// BodyNullPolicyLocator recognizes body null in its native source decoder.
// The policy is invocation-local: implementations must never mutate a provider
// or treat an arbitrary provider's nil value as evidence of JSON literal null.
type BodyNullPolicyLocator interface {
	ValueWithBodyNullPolicy(context.Context, reflect.Type, string, string) (any, bool, error)
}

// PresenceLocator reports source suppliedness without interpreting its value.
// Implementations retain ordinary source syntax, name selection and ambiguity rules.
type PresenceLocator interface {
	Presence(context.Context, string) (bool, error)
}

// BodyInputTagLocator consumes a whole-body native input policy without
// mutating the shared source or applying the policy to each element.
type BodyInputTagLocator interface {
	ValueWithBodyInputTag(context.Context, reflect.Type, string, reflect.StructTag) (any, bool, error)
}

// BodyRepresentationLocator overlays immutable metadata for one body lookup.
// The source retains decoder ownership, admission policy, selection and bytes.
type BodyRepresentationLocator interface {
	ValueWithBodyRepresentation(context.Context, reflect.Type, string, *shape.Representation, string, reflect.StructTag) (any, bool, error)
}
