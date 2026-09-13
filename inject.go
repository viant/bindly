package bindly

import (
	"context"
	"github.com/viant/bindly/state"
)

// Inject uses the same immutable-plan invocation path as Injector.Bind.
func (c *BindingContext[T]) Inject(ctx context.Context, target *T) error {
	source := c.state.StatePtr()
	if source == nil {
		source = c.state.State()
	}
	return c.injector.Bind(ctx, target, WithSource(source), func(options *bindOptions) { options.cache = c.valueCache })
}

func (c *BindingContext[T]) Value(ctx context.Context, location *state.Location) (any, bool, error) {
	scope := &invocation{injector: c.injector, source: c.state, active: map[string]bool{}, persistent: c.valueCache}
	return scope.Value(ctx, location)
}
