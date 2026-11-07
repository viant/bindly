package body

import (
	"github.com/viant/bindly/input"
	"github.com/viant/bindly/internal/bodyjson"
	"github.com/viant/x/shape"
	"reflect"
)

// ValidateRequired checks explicit nested required-field presence before native
// decoding. It does not change Source.Value defaults. Hidden fields and opaque
// custom representations retain their existing owner boundaries. Null, zero and
// false are supplied values; this function does not perform value validation.
func ValidateRequired(raw []byte, target reflect.Type, metadata ...*shape.Representation) error {
	if err := bodyjson.ValidateRequired(raw, target, metadata...); err != nil {
		return &input.Error{Cause: err}
	}
	return nil
}
