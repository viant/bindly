package body

import (
	"encoding/json"
	"github.com/viant/bindly/internal/bodyjson"
	"reflect"
)

func (s *Source) markPresence(target reflect.Value, raw json.RawMessage) error {
	return bodyjson.MarkPresence(target, raw, true)
}
