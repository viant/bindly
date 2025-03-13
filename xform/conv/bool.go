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

// BoolTransformer converts compatible values to bool
type BoolTransformer struct {
	xform.TransformerBase
}

func (t *BoolTransformer) Transform(ctx context.Context, resolver locator.Resolver, input interface{}) (interface{}, error) {
	if input == nil {
		return false, nil
	}

	// Convert input to bool based on its type
	switch v := input.(type) {
	case bool:
		return v, nil
	case string:
		switch v {
		case "true", "yes", "1", "on":
			return true, nil
		case "false", "no", "0", "off", "":
			return false, nil
		default:
			return nil, fmt.Errorf("cannot convert string '%s' to bool", v)
		}
	case int, int32, int64:
		return reflect.ValueOf(v).Int() != 0, nil
	default:
		return nil, fmt.Errorf("cannot convert %T to bool", input)
	}
}

// NewBoolTransformer creates a new bool transformer
func NewBoolTransformer(ctx context.Context, config tags.Values, destType reflect.Type, embedFS *embed.FS) (xform.Transformer, error) {
	if destType.Kind() != reflect.Bool {
		return nil, fmt.Errorf("BoolTransformer can only be used with bool destination type, got %v", destType)
	}

	return &BoolTransformer{
		TransformerBase: xform.NewTransformerBase("bool", destType, config, embedFS),
	}, nil
}
