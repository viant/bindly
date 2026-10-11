package bodyjson

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// This private default preserves Datly69de's time/time-pointer unmarshalling
// and Tagly v0.3.1-0.20260914020630-eadee36c3630 format/time.Parse. It intentionally
// does not call the newer Tagly parser, which changed fallback behavior.
func normalizeLegacyTime(raw []byte, pointer bool, dates *dateState, path string) ([]byte, error) {
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	stamp, err := parseLegacyTime(time.RFC3339, strings.TrimRight(value, "Z"))
	if err != nil {
		return nil, err
	}
	// The old pointer marshaller did not assign a parsed zero to a fresh pointer.
	if pointer && stamp.IsZero() {
		if previous := dates.values[path]; previous != nil {
			return previous, nil
		}
		return []byte("null"), nil
	}
	normalized, err := json.Marshal(stamp.Format(time.RFC3339Nano))
	if err == nil && pointer {
		dates.values[path] = normalized
	}
	return normalized, err
}

// Source-faithful parser; the one bounds check hardens a legacy malformed
// prefix slice panic into an ordinary body input error.
func parseLegacyTime(layout, value string) (time.Time, error) {
	if layout == "" {
		layout = time.RFC3339 //TODO add layout autodetection for this case
	}

	if strings.Contains(value, ".") {
		if layout == time.RFC3339 {
			layout = time.RFC3339Nano
		}
	}

	//adjust T fragment
	if strings.Contains(value, "T") != strings.Contains(layout, "T") {
		layout = strings.Replace(layout, "T", " ", 1)
		value = strings.Replace(value, "T", " ", 1)
	}
	t, err := time.ParseInLocation(layout, value, time.UTC)

	originalLayout := layout

	if err != nil {

		if len(value) > len(layout) {
			value = value[:len(layout)]
			t, err = time.Parse(layout, value)
		} else {
			layout = layout[:len(value)]
			t, err = time.Parse(layout, value)
		}

		parseErr, ok := err.(*time.ParseError)
		if ok && err != nil {
			if parseErr.LayoutElem != "" {
				if index := strings.LastIndex(originalLayout, parseErr.LayoutElem); index != -1 {
					if index > len(value) {
						return time.Time{}, fmt.Errorf("legacy date prefix exceeds input length")
					}
					value = value[:index]
					layout = originalLayout[:index]
					t, err = time.Parse(layout, value)
				}
			}
		}

		if err != nil {
			layout = "2006-01-02-07:00"
			t, err = time.Parse(layout, value)
		}
	}
	return t, err
}

// This call-local lexical history mirrors native pointer reuse for duplicate
// object members. It is neither a typed body nor a second JSON traversal.
type dateState struct{ values map[string][]byte }

func (s *dateState) clear(path string) {
	for key := range s.values {
		if key == path || strings.HasPrefix(key, path+"/") {
			delete(s.values, key)
		}
	}
}
