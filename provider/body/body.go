// Package body resolves whole or named request bodies to planned Go types.
package body

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/viant/bindly/input"
	"github.com/viant/bindly/internal/bodyjson"
	"mime"
	"mime/multipart"
	"reflect"
	"strings"

	"github.com/viant/bindly/locator"
	"github.com/viant/bindly/xform/conv"
	"github.com/viant/structology"
)

type Source struct {
	deferred  *deferredSource
	raw       []byte
	mediaType string
	exact     bool
	aliases   map[string]string
	decoders  *DecoderRegistry
	multipart *multipart.Form
}
type Option func(*Source)

func WithExactFieldNames() Option { return func(s *Source) { s.exact = true } }
func New(raw []byte, contentType string, aliases map[string]string, options ...Option) (*Source, error) {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil && contentType != "" {
		return nil, &input.Error{Cause: err}
	}
	s := &Source{raw: append([]byte(nil), raw...), mediaType: mediaType, aliases: map[string]string{}}
	for name, alias := range aliases {
		s.aliases[name] = alias
	}
	for _, option := range options {
		if option != nil {
			option(s)
		}
	}
	return s, nil
}
func (s *Source) Kind() string                              { return "body" }
func (s *Source) Priority() int                             { return 0 }
func (s *Source) DefaultCacheable() bool                    { return true }
func (s *Source) Locate(*structology.State) locator.Locator { return s }
func (s *Source) Value(ctx context.Context, target reflect.Type, name string) (any, bool, error) {
	return s.ValueWithBodyNullPolicy(ctx, target, name, "")
}

// ValueWithBodyNullPolicy applies only this binding's whole JSON null policy.
func (s *Source) ValueWithBodyNullPolicy(ctx context.Context, target reflect.Type, name, policy string) (any, bool, error) {
	if policy != "" {
		if policy != "empty-record" || name != "" || target == nil || target.Kind() != reflect.Pointer || target.Elem().Kind() != reflect.Struct {
			return nil, false, &input.Error{Cause: fmt.Errorf("invalid whole-body null policy %q for %v/%q", policy, target, name)}
		}
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}
	}
	if s != nil && s.deferred != nil {
		resolved, err := s.resolve(ctx)
		if err != nil {
			return nil, false, err
		}
		return resolved.ValueWithBodyNullPolicy(ctx, target, name, policy)
	}
	if s == nil {
		return nil, false, nil
	}
	raw := s.raw
	if alias, ok := s.aliases[name]; ok {
		name = alias
	}
	if policy != "" && name != "" {
		return nil, false, &input.Error{Cause: fmt.Errorf("bodyNullPolicy cannot select an aliased named body field")}
	}
	if strings.HasPrefix(s.mediaType, "multipart/") {
		value, found := MultipartValue(s.multipart, target, name)
		if name == "" {
			return nil, false, &input.Error{Cause: fmt.Errorf("multipart body binding requires a field name")}
		}
		return value, found, nil
	}
	if len(raw) == 0 {
		return nil, false, nil
	}
	if s.mediaType == "application/json" || strings.HasSuffix(s.mediaType, "+json") || s.mediaType == "" {
		if name != "" {
			var object map[string]json.RawMessage
			if err := json.Unmarshal(raw, &object); err != nil {
				return nil, false, &input.Error{Cause: err}
			}
			var ok bool
			raw, ok = object[name]
			if !ok && !s.exact {
				for key, value := range object {
					if strings.EqualFold(key, name) {
						if ok {
							return nil, false, &input.Error{Cause: fmt.Errorf("ambiguous body field %q", name)}
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
			if policy == "empty-record" {
				var literal json.RawMessage
				if err := json.Unmarshal(raw, &literal); err != nil {
					return nil, true, &input.Error{Cause: err}
				}
				value, err := NewEmptyRecord(target)
				return value, true, err
			}
			return nil, true, nil
		}
		if target == nil {
			var value any
			err := json.Unmarshal(raw, &value)
			if err != nil {
				return nil, true, &input.Error{Cause: err}
			}
			return value, true, nil
		}
		if target == reflect.TypeOf(json.RawMessage{}) {
			return append(json.RawMessage(nil), raw...), true, nil
		}
		if _, found := s.decoders.Lookup(s.mediaType); !found {
			value, err := bodyjson.DecodePublic(raw, target)
			return value, true, err
		}
		filtered, err := bodyjson.FilterPublic(raw, target)
		if err != nil {
			return nil, true, &input.Error{Cause: err}
		}
		raw = filtered
		value := reflect.New(target)
		decode := json.Unmarshal
		if decoder, found := s.decoders.Lookup(s.mediaType); found {
			decode = decoder.Decode
		}
		if err := decode(raw, value.Interface()); err != nil {
			return nil, true, &input.Error{Cause: err}
		}
		if err := s.markPresence(value.Elem(), raw); err != nil {
			return nil, true, err
		}
		return value.Elem().Interface(), true, nil
	}
	if decoder, found := s.decoders.Lookup(s.mediaType); found {
		if target == nil {
			return nil, false, &input.Error{Cause: fmt.Errorf("body binding source type is required")}
		}
		value := reflect.New(target)
		if name == "" {
			if err := decoder.Decode(raw, value.Interface()); err != nil {
				return nil, false, &input.Error{Cause: err}
			}
			return value.Elem().Interface(), true, nil
		}
		fields, supported := decoder.(FieldDecoder)
		if !supported {
			return nil, false, &input.Error{Cause: fmt.Errorf("body decoder for %q does not support named fields", s.mediaType)}
		}
		if s.exact {
			return nil, false, &input.Error{Cause: fmt.Errorf("exact named body lookup is unsupported for %q", s.mediaType)}
		}
		found, err := fields.DecodeField(raw, name, value.Interface())
		if err != nil {
			return nil, found, &input.Error{Cause: err}
		}
		if !found {
			return nil, false, nil
		}
		return value.Elem().Interface(), true, nil
	}
	if name != "" {
		return nil, false, &input.Error{Code: 415, Cause: fmt.Errorf("named fields require a JSON body")}
	}
	valueType := target
	for valueType != nil && valueType.Kind() == reflect.Pointer {
		valueType = valueType.Elem()
	}
	converter := conv.ValueConverter{DisallowJSON: true}
	if valueType == reflect.TypeOf([]byte{}) {
		value, err := converter.Convert(append([]byte(nil), raw...), target)
		if err != nil {
			return nil, true, &input.Error{Cause: err}
		}
		return value, true, nil
	}
	value, err := converter.Convert(string(raw), target)
	if err != nil {
		return nil, true, &input.Error{Cause: err}
	}
	return value, true, nil
}

// NewEmptyRecord allocates a detached direct *struct and initializes native
// original-presence holders without marking any business field as supplied.
// Callers normalizing typed input must independently prove explicit presence.
func NewEmptyRecord(target reflect.Type) (any, error) {
	if target == nil || target.Kind() != reflect.Pointer || target.Elem().Kind() != reflect.Struct {
		return nil, fmt.Errorf("empty record requires a direct pointer-to-struct target")
	}
	value := reflect.New(target.Elem())
	if err := (&Source{}).markPresence(value, json.RawMessage(`{}`)); err != nil {
		return nil, err
	}
	return value.Interface(), nil
}
