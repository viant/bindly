package xform

import (
	"context"
	"github.com/viant/bindly/locator"
)

type Transformer interface {
	Transform(ctx context.Context, resolver locator.Resolver, input interface{}) (interface{}, error)
}
