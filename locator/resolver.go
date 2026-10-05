package locator

import (
	"context"
	"reflect"

	"github.com/viant/bindly/state"
)

type Resolver interface {
	Value(ctx context.Context, location *state.Location) (interface{}, bool, error)
}

// Scope is the active binding scope supplied to providers whose values depend
// on other invocation-local dependencies.
type Scope interface {
	Resolver
	BindTarget(ctx context.Context, target interface{}) error
}

// ScopedLocator resolves a value with access to the active binding scope.
// Ordinary locators should continue to implement Locator only.
type ScopedLocator interface {
	Locator
	ValueInScope(ctx context.Context, scope Scope, targetType reflect.Type, name string) (interface{}, bool, error)
}
