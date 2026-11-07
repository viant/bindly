package bindly

import "context"

// Binder binds a typed target using the implementation's provider scope.
// Calls without options retain the implementation's normal binding behavior.
// Specialized binders may restrict options to preserve their source, plan or
// invocation authority; unsupported options must return an error, not be ignored.
type Binder interface {
	Bind(ctx context.Context, target any, options ...BindOption) error
}

var _ Binder = (*Injector)(nil)
