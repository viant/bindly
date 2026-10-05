// Package bodyjson shares native typed JSON and presence reconstruction inside Bindly.
package bodyjson

import (
	"encoding/json"
	"github.com/viant/bindly/input"
	"reflect"
	"strings"
)

// DecodePublic excludes canonical internal members before invoking JSON decoders.
func DecodePublic(raw []byte, target reflect.Type) (any, error) { return decode(raw, target, true) }

// VerifyTypedRoundtrip reconstructs only Replay.encodeField's immediately preceding
// local marshal. Stored raw replay continues through the public body Source.
func VerifyTypedRoundtrip(raw []byte, target reflect.Type) (any, error) {
	return decode(raw, target, false)
}

func decode(raw []byte, target reflect.Type, public bool) (any, error) {
	if strings.TrimSpace(string(raw)) == "null" {
		return nil, nil
	}
	if target == nil {
		var value any
		if err := json.Unmarshal(raw, &value); err != nil {
			return nil, &input.Error{Cause: err}
		}
		return value, nil
	}
	if target == reflect.TypeOf(json.RawMessage{}) {
		return append(json.RawMessage(nil), raw...), nil
	}
	if public {
		filtered, err := excludeInternal(raw, target)
		if err != nil {
			return nil, &input.Error{Cause: err}
		}
		raw = filtered
	}
	value := reflect.New(target)
	if err := json.Unmarshal(raw, value.Interface()); err != nil {
		return nil, &input.Error{Cause: err}
	}
	if err := MarkPresence(value.Elem(), raw, public); err != nil {
		return nil, err
	}
	return value.Elem().Interface(), nil
}

// FilterPublic shares canonical JSON filtering with a registered JSON decoder.
// The caller retains its existing decoder and native presence reconstruction.
func FilterPublic(raw []byte, target reflect.Type) ([]byte, error) {
	return excludeInternal(raw, target)
}
