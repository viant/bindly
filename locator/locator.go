package locator

import (
	"context"
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
