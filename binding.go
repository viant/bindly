package bindly

import (
	"github.com/viant/bindly/locator"
	"github.com/viant/bindly/state"
	"github.com/viant/bindly/xform"
	"github.com/viant/structology"
	"github.com/viant/tagly/tags"
)

// Binding represents a binding
type Binding struct {
	selector     *structology.Selector
	location     *state.Location
	provider     locator.Provider
	cachable     bool
	required     bool
	defaultValue interface{}
	transformer  xform.Transformer
	xformConfig  tags.Values
}
