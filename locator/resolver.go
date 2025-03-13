package locator

import (
	"context"
	"github.com/viant/bindly/state"
)

type Resolver interface {
	Value(ctx context.Context, location *state.Location) (interface{}, bool, error)
}
