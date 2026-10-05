package xform

import (
	"fmt"
	"reflect"

	"github.com/viant/bindly/resource"
	"github.com/viant/tagly/tags"
)

// TransformerBase provides basic transformer functionality
type TransformerBase struct {
	name      string
	destType  reflect.Type
	config    tags.Values
	resources *resource.Store
}

// NewTransformerBase creates a new transformer base
func NewTransformerBase(name string, destType reflect.Type, config tags.Values, resources *resource.Store) TransformerBase {
	return TransformerBase{
		name:      name,
		destType:  destType,
		config:    config,
		resources: resources,
	}
}

func (b *TransformerBase) ReadResource(reference string) ([]byte, error) {
	if b == nil || b.resources == nil {
		return nil, fmt.Errorf("transformer resource store is not configured")
	}
	return b.resources.ReadFile(reference)
}
