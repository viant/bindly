package bindly

import (
	"context"
	"fmt"
	"github.com/viant/bindly/state"
	"github.com/viant/bindly/xform/conv"
	"github.com/viant/structology"
)

// Inject uses the same immutable-plan invocation path as Injector.Bind.
func (c *BindingContext[T]) Inject(ctx context.Context, target *T) error {
	source := c.state.StatePtr()
	if source == nil {
		source = c.state.State()
	}
	return c.injector.Bind(ctx, target, WithSource(source), c.bindOption())
}

func (c *BindingContext[T]) Value(ctx context.Context, location *state.Location) (any, bool, error) {
	scope := &invocation{injector: c.injector, source: c.state, active: map[string]bool{}, persistent: c.valueCache, strictMissing: c.strictMissing, allowedKinds: c.allowedKinds, delayedKinds: c.delayedKinds}
	return scope.Value(ctx, location)
}

func (c *BindingContext[T]) bindOption() BindOption {
	return func(o *bindOptions) {
		o.cache = c.valueCache
		o.allowedKinds = c.allowedKinds
		o.delayedKinds = c.delayedKinds
		o.strictMissing = c.strictMissing
	}
}
func (c *BindingContext[T]) Bind(ctx context.Context) error {
	return c.BindTarget(ctx, c.state.StatePtr())
}
func (c *BindingContext[T]) BindTarget(ctx context.Context, target any) error {
	if c == nil || c.injector == nil {
		return fmt.Errorf("binding scope is required")
	}
	source := c.state.StatePtr()
	if source == nil {
		source = c.state.State()
	}
	return c.injector.Bind(ctx, target, WithSource(source), c.bindOption())
}
func (c *BindingContext[T]) adjustValue(selector *structology.Selector, value any) (any, error) {
	return (conv.ValueConverter{}).Convert(value, selector.Type())
}
