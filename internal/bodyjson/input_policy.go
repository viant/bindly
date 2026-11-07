package bodyjson

import (
	"bytes"
	"encoding/json"
	"github.com/viant/tagly/format"
)

type inputScopeState struct {
	enabled   bool
	ancestors []format.InputScope
}

func selectInputMembers(raw []byte, names *format.InputNameMatcher, ancestors []format.InputScope) ([]bool, []format.InputMember, error) {
	d := json.NewDecoder(bytes.NewReader(raw))
	if _, err := d.Token(); err != nil {
		return nil, nil, err
	}
	var members []format.InputMember
	for d.More() {
		key, err := d.Token()
		if err != nil {
			return nil, nil, err
		}
		var value json.RawMessage
		if err = d.Decode(&value); err != nil {
			return nil, nil, err
		}
		members = append(members, format.InputMember{Name: key.(string), Kind: inputMemberKind(value)})
	}
	if _, err := d.Token(); err != nil {
		return nil, nil, err
	}
	selected, err := names.SelectScoped(members, ancestors)
	return selected, members, err
}
func inputMemberKind(raw []byte) string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return ""
	}
	switch raw[0] {
	case '{':
		return "object"
	case '[':
		return "array"
	case '"':
		return "string"
	case 'n':
		return "null"
	case 't', 'f':
		return "boolean"
	default:
		return "number"
	}
}
