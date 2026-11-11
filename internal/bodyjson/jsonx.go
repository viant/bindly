package bodyjson

import (
	"fmt"
	"reflect"
	"strings"

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

// ValidateRootInputTag bounds root adaptation to the native record collection
// policy. Unlike generic field jsonx, an unknown root option is never ignored.
func ValidateRootInputTag(tag reflect.StructTag, target reflect.Type) error {
	if tag == "" {
		return nil
	}
	object := false
	for _, option := range strings.Split(tag.Get("jsonx"), ",") {
		switch strings.TrimSpace(option) {
		case "objectToSlice":
			object = true
		case "null=omit":
		default:
			return fmt.Errorf("unsupported whole-body jsonx option %q", option)
		}
	}
	if !object {
		return fmt.Errorf("whole-body jsonx requires objectToSlice")
	}
	_, err := compileInputRule(tag, target)
	return err
}

// ApplyRootInputTag runs the same native policy as admitted record members.
// Its result must feed filtering, typed decoding and presence together.
func ApplyRootInputTag(raw []byte, tag reflect.StructTag, target reflect.Type) ([]byte, bool, error) {
	if err := ValidateRootInputTag(tag, target); err != nil {
		return nil, false, err
	}
	rule, err := compileInputRule(tag, target)
	if err != nil {
		return nil, false, err
	}
	return rule.apply(raw)
}
