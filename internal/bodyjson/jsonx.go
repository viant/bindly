package bodyjson

import (
	"fmt"
	"reflect"

	"github.com/viant/tagly/jsonx"
)

// inputRule augments native decoding only on explicitly authored fields.
type inputRule struct{ policy *jsonx.Policy }

func compileInputRule(tag reflect.StructTag, typ reflect.Type) (inputRule, error) {
	policy, err := jsonx.Compile(tag, typ)
	if err != nil {
		return inputRule{}, err
	}
	if policy.Active() && ownsJSONRepresentation(typ) {
		return inputRule{}, fmt.Errorf("jsonx input rules cannot override custom JSON ownership on %v", typ)
	}
	return inputRule{policy: policy}, nil
}
func (r inputRule) active() bool                           { return r.policy.Active() }
func (r inputRule) apply(raw []byte) ([]byte, bool, error) { return r.policy.Apply(raw) }
