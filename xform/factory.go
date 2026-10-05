package xform

import (
	"context"
	"reflect"

	"github.com/viant/bindly/resource"
	"github.com/viant/tagly/tags"
)

type Factory interface {
	Create(ctx context.Context, config tags.Values, destType reflect.Type, resources *resource.Store) (Transformer, error)
}

// transformerFactory is a base factory for transformers
type transformerFactory struct {
	constructor func(ctx context.Context, config tags.Values, destType reflect.Type, resources *resource.Store) (Transformer, error)
	name        string
}

func (f *transformerFactory) Create(ctx context.Context, config tags.Values, destType reflect.Type, resources *resource.Store) (Transformer, error) {
	return f.constructor(ctx, config, destType, resources)
}

// NewTransformerFactory creates a new transformer factory
func NewTransformerFactory(name string, constructor func(ctx context.Context, config tags.Values, destType reflect.Type, resources *resource.Store) (Transformer, error)) Factory {
	return &transformerFactory{
		name:        name,
		constructor: constructor,
	}
}
