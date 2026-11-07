// Package bodyjson shares native typed JSON and presence reconstruction inside Bindly.
package bodyjson

import (
	"encoding/json"
	"github.com/viant/bindly/input"
	structjson "github.com/viant/structology/encoding/json"
	"github.com/viant/tagly/choice"
	xshape "github.com/viant/x/shape"
	"reflect"
	"strings"
)

// DecodePublic filters canonical internal members and normalizes default dates
// before the existing typed JSON decoder and presence reconstruction.
func DecodePublic(raw []byte, target reflect.Type, metadata ...*xshape.Representation) (any, error) {
	return decode(raw, target, true, metadata...)
}

// VerifyTypedRoundtrip reconstructs only Replay.encodeField's immediately preceding
// local marshal. Stored raw replay continues through the public body Source.
func VerifyTypedRoundtrip(raw []byte, target reflect.Type) (any, error) {
	return decode(raw, target, false)
}

func decode(raw []byte, target reflect.Type, public bool, metadata ...*xshape.Representation) (any, error) {
	representation := metadataForType(firstMetadata(metadata), target)
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
		filtered, err := filterPublic(raw, target, true, representation)
		if err != nil {
			return nil, &input.Error{Cause: err}
		}
		raw = filtered
	}
	value := reflect.New(target)
	decode := json.Unmarshal
	declaredDates := false
	aliases := false
	if public {
		plan, planErr := compileInternalPlan(target, nil, representation)
		if planErr != nil {
			return nil, &input.Error{Cause: planErr}
		}
		declaredDates = plan != nil && plan.declaredDates
		aliases = plan != nil && plan.aliases
	}
	if representation != nil || choice.HasEnum(target) || declaredDates || aliases {
		decode = func(data []byte, target any) error {
			return structjson.UnmarshalStandard(data, target, structjson.WithRepresentation(representation))
		}
	}
	if err := decode(raw, value.Interface()); err != nil {
		enrichArrayTypeError(raw, target, err)
		return nil, &input.Error{Cause: err}
	}
	if err := MarkPresence(value.Elem(), raw, public, representation); err != nil {
		return nil, err
	}
	return value.Elem().Interface(), nil
}

// FilterPublic shares canonical JSON filtering with a registered JSON decoder.
// The caller retains its existing decoder and native presence reconstruction.
func FilterPublic(raw []byte, target reflect.Type, metadata ...*xshape.Representation) ([]byte, error) {
	return filterPublic(raw, target, false, metadata...)
}
