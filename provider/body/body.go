// Package body resolves whole or named request bodies to planned Go types.
package body

import (
	"context"
	"encoding/json"
	"fmt"
	"mime"
	"reflect"
	"strings"

	"github.com/viant/bindly/locator"
	"github.com/viant/bindly/xform/conv"
	"github.com/viant/structology"
)

type Source struct {
	raw       []byte
	mediaType string
	exact     bool
	aliases   map[string]string
}
type Option func(*Source)

func WithExactFieldNames() Option { return func(s *Source) { s.exact = true } }
func New(raw []byte, contentType string, aliases map[string]string, options ...Option) (*Source, error) {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil && contentType != "" {
		return nil, err
	}
	s := &Source{raw: append([]byte(nil), raw...), mediaType: mediaType, aliases: map[string]string{}}
	for name, alias := range aliases {
		s.aliases[name] = alias
	}
	for _, option := range options {
		option(s)
	}
	return s, nil
}
func (s *Source) Kind() string                              { return "body" }
func (s *Source) Priority() int                             { return 0 }
func (s *Source) DefaultCacheable() bool                    { return true }
func (s *Source) Locate(*structology.State) locator.Locator { return s }
func (s *Source) Value(_ context.Context, target reflect.Type, name string) (any, bool, error) {
	if s == nil || len(s.raw) == 0 {
		return nil, false, nil
	}
	raw := s.raw
	if alias, ok := s.aliases[name]; ok {
		name = alias
	}
	if s.mediaType == "application/json" || strings.HasSuffix(s.mediaType, "+json") || s.mediaType == "" {
		if name != "" {
			var object map[string]json.RawMessage
			if err := json.Unmarshal(raw, &object); err != nil {
				return nil, false, err
			}
			var ok bool
			raw, ok = object[name]
			if !ok && !s.exact {
				for key, value := range object {
					if strings.EqualFold(key, name) {
						if ok {
							return nil, false, fmt.Errorf("ambiguous body field %q", name)
						}
						raw, ok = value, true
					}
				}
			}
			if !ok {
				return nil, false, nil
			}
		}
		if strings.TrimSpace(string(raw)) == "null" {
			return nil, true, nil
		}
		if target == nil {
			var value any
			err := json.Unmarshal(raw, &value)
			return value, true, err
		}
		if target == reflect.TypeOf(json.RawMessage{}) {
			return append(json.RawMessage(nil), raw...), true, nil
		}
		value := reflect.New(target)
		if err := json.Unmarshal(raw, value.Interface()); err != nil {
			return nil, true, err
		}
		if err := s.markPresence(value.Elem(), raw); err != nil {
			return nil, true, err
		}
		return value.Elem().Interface(), true, nil
	}
	if name != "" {
		return nil, false, fmt.Errorf("named fields require a JSON body")
	}
	valueType := target
	for valueType != nil && valueType.Kind() == reflect.Pointer {
		valueType = valueType.Elem()
	}
	converter := conv.ValueConverter{DisallowJSON: true}
	if valueType == reflect.TypeOf([]byte{}) {
		value, err := converter.Convert(append([]byte(nil), raw...), target)
		return value, true, err
	}
	value, err := converter.Convert(string(raw), target)
	return value, true, err
}
