package conv

import "github.com/viant/bindly/xform"

// Init standard transformers
func Init(registry *xform.Registry) {
	registry.Register("string", xform.NewTransformerFactory("string", NewStringTransformer))
	registry.Register("int", xform.NewTransformerFactory("int", NewIntTransformer))
	registry.Register("bool", xform.NewTransformerFactory("bool", NewBoolTransformer))
}
