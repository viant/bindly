package body

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/viant/bindly/input"
	"strings"
)

// selectJSON is the single named JSON selection path for values and presence.
// The caller has resolved the configured source alias. Unmarshal validates all
// document syntax while RawMessage preserves arbitrary valid numeric lexemes.
func (s *Source) selectJSON(name string) (json.RawMessage, bool, error) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(s.raw, &object); err != nil {
		return nil, false, err
	}
	raw, found := object[name]
	if !found && !s.exact {
		for key, value := range object {
			if strings.EqualFold(key, name) {
				if found {
					return nil, false, fmt.Errorf("ambiguous body field %q", name)
				}
				raw, found = value, true
			}
		}
	}
	return raw, found, nil
}

// Presence reports named source suppliedness without decoding the selected value.
func (s *Source) Presence(ctx context.Context, name string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if s != nil && s.deferred != nil {
		resolved, err := s.resolve(ctx)
		if err != nil {
			return false, err
		}
		return resolved.Presence(ctx, name)
	}
	if s == nil {
		return false, nil
	}
	if alias, ok := s.aliases[name]; ok {
		name = alias
	}
	if name == "" {
		return false, &input.Error{Cause: fmt.Errorf("presenceOnly requires a named JSON body field")}
	}
	if s.mediaType != "" && s.mediaType != "application/json" && !strings.HasSuffix(s.mediaType, "+json") {
		return false, &input.Error{Cause: fmt.Errorf("presenceOnly requires JSON media")}
	}
	if len(s.raw) == 0 {
		return false, nil
	}
	_, found, err := s.selectJSON(name)
	if err != nil {
		return false, &input.Error{Cause: err}
	}
	return found, nil
}
