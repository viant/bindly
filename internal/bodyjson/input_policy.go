package bodyjson

import (
	"bytes"
	"encoding/json"
	"github.com/viant/tagly/format"
)

func selectInputMembers(raw []byte, names *format.InputNameMatcher) ([]bool, error) {
	d := json.NewDecoder(bytes.NewReader(raw))
	if _, err := d.Token(); err != nil {
		return nil, err
	}
	var members []format.InputMember
	for d.More() {
		key, err := d.Token()
		if err != nil {
			return nil, err
		}
		var value json.RawMessage
		if err = d.Decode(&value); err != nil {
			return nil, err
		}
		members = append(members, format.InputMember{Name: key.(string), Kind: inputMemberKind(value)})
	}
	if _, err := d.Token(); err != nil {
		return nil, err
	}
	return names.Select(members), nil
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
