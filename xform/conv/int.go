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

// IntTransformer converts compatible values to int
type IntTransformer struct {
	xform.TransformerBase
}

func (t *IntTransformer) Transform(ctx context.Context, resolver locator.Resolver, input interface{}) (interface{}, error) {
	if input == nil {
		return 0, nil
	}

	// Convert input to int based on its type
	switch v := input.(type) {
	case int:
		return v, nil
	case int32:
		return int(v), nil
	case int64:
		return int(v), nil
	case float32:
		return int(v), nil
	case float64:
		return int(v), nil
	case string:
		var result int
		if _, err := fmt.Sscanf(v, "%d", &result); err != nil {
			return nil, fmt.Errorf("failed to convert string to int: %v", err)
		}
		return result, nil
	default:
		return nil, fmt.Errorf("cannot convert %T to int", input)
	}
}

// NewIntTransformer creates a new int transformer
func NewIntTransformer(ctx context.Context, config tags.Values, destType reflect.Type, embedFS *embed.FS) (xform.Transformer, error) {
	if destType.Kind() != reflect.Int && destType.Kind() != reflect.Int32 && destType.Kind() != reflect.Int64 {
		return nil, fmt.Errorf("IntTransformer can only be used with int destination types, got %v", destType)
	}
	return &IntTransformer{
		TransformerBase: xform.NewTransformerBase("int", destType, config, embedFS),
	}, nil
}
