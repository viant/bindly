package xform

import (
	"embed"
	"github.com/viant/tagly/tags"
	"reflect"
)

// TransformerBase provides basic transformer functionality
type TransformerBase struct {
	name     string
	destType reflect.Type
	config   tags.Values
	embedFS  *embed.FS
}

// NewTransformerBase creates a new transformer base
func NewTransformerBase(name string, destType reflect.Type, config tags.Values, embedFS *embed.FS) TransformerBase {
	return TransformerBase{
		name:     name,
		destType: destType,
		config:   config,
		embedFS:  embedFS,
	}
}
