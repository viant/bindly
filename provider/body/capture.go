package body

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
)

// CaptureSource retains the selected JSON bytes before decoding loses omission,
// null or number precision. Other media keep this provider's normal source-type
// conversion. ReplayPlan owns selection; the body provider owns JSON extraction.
func (s *Source) CaptureSource(ctx context.Context, target reflect.Type, name string) (any, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	if s != nil && (s.mediaType == "" || s.mediaType == "application/json" || strings.HasSuffix(s.mediaType, "+json")) {
		target = reflect.TypeFor[json.RawMessage]()
	}
	return s.Value(ctx, target, name)
}
