package conv

import (
	"context"
	"embed"
	"fmt"
	"github.com/viant/bindly/locator"
	"github.com/viant/bindly/xform"
	"github.com/viant/tagly/tags"
	"reflect"
)

// StringTransformer converts any value to a string
type StringTransformer struct {
	xform.TransformerBase
}

func (t *StringTransformer) Transform(ctx context.Context, resolver locator.Resolver, input interface{}) (interface{}, error) {
	if input == nil {
		return "", nil
	}

	// Convert input to string based on its type
	switch v := input.(type) {
	case string:
		return v, nil
	case []byte:
		return string(v), nil
	case fmt.Stringer:
		return v.String(), nil
	default:
		return fmt.Sprintf("%v", v), nil
	}
}

// NewStringTransformer creates a new string transformer
func NewStringTransformer(ctx context.Context, config tags.Values, destType reflect.Type, embedFS *embed.FS) (xform.Transformer, error) {
	if destType.Kind() != reflect.String {
		return nil, fmt.Errorf("StringTransformer can only be used with string destination type, got %v", destType)
	}
	return &StringTransformer{
		TransformerBase: xform.NewTransformerBase("string", destType, config, embedFS),
	}, nil
}
