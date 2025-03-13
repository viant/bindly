package xform

import (
	"context"
	"embed"
	"github.com/viant/tagly/tags"
	"reflect"
)

type Factory interface {
	Create(ctx context.Context, config tags.Values, destType reflect.Type, embedFS *embed.FS) (Transformer, error)
}

// transformerFactory is a base factory for transformers
type transformerFactory struct {
	constructor func(ctx context.Context, config tags.Values, destType reflect.Type, embedFS *embed.FS) (Transformer, error)
	name        string
}

func (f *transformerFactory) Create(ctx context.Context, config tags.Values, destType reflect.Type, embedFS *embed.FS) (Transformer, error) {
	return f.constructor(ctx, config, destType, embedFS)
}

// NewTransformerFactory creates a new transformer factory
func NewTransformerFactory(name string, constructor func(ctx context.Context, config tags.Values, destType reflect.Type, embedFS *embed.FS) (Transformer, error)) Factory {
	return &transformerFactory{
		name:        name,
		constructor: constructor,
	}
}
