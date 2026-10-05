package body

import (
	"fmt"
	"reflect"
	"strings"
)

type Option func(*Source)

// WithExactFieldNames makes named JSON body lookup fail closed after an exact
// key miss. It is intended for compiled non-HTTP protocol adapters such as MCP.
func WithExactFieldNames() Option {
	return func(source *Source) {
		source.exactFieldNames = true
	}
}

// Source exposes immutable raw body bytes through typed decoder contracts.
type Source struct {
	raw             []byte
	mediaType       string
	decoders        *DecoderRegistry
	exactFieldNames bool
}

func New(raw []byte, mediaType string, decoders *DecoderRegistry, options ...Option) (*Source, error) {
	mediaType = strings.ToLower(strings.TrimSpace(mediaType))
	if mediaType == "" {
		return nil, fmt.Errorf("body media type is required")
	}
	if decoders == nil {
		decoders = NewDecoderRegistry()
	}
	result := &Source{
		raw:       append([]byte(nil), raw...),
		mediaType: mediaType,
		decoders:  decoders,
	}
	for _, option := range options {
		if option != nil {
			option(result)
		}
	}
	return result, nil
}

func (s *Source) Value(targetType reflect.Type, name string) (interface{}, bool, error) {
	if s == nil {
		return nil, false, nil
	}
	if targetType == nil {
		return nil, false, fmt.Errorf("body binding source type is required")
	}
	if len(s.raw) == 0 {
		return nil, false, nil
	}
	if name == "" {
		switch {
		case targetType == reflect.TypeOf([]byte{}):
			return append([]byte(nil), s.raw...), true, nil
		case targetType.Kind() == reflect.String:
			return string(s.raw), true, nil
		}
	}
	decoder, ok := s.decoders.Lookup(s.mediaType)
	if !ok {
		return nil, false, fmt.Errorf("no body decoder registered for %q", s.mediaType)
	}
	target, result := decodeTarget(targetType)
	if name == "" {
		if err := decoder.Decode(s.raw, target.Interface()); err != nil {
			return nil, false, fmt.Errorf("decode %s body: %w", s.mediaType, err)
		}
		return result(), true, nil
	}
	if s.exactFieldNames {
		if s.mediaType != "application/json" {
			return nil, false, fmt.Errorf("exact named body lookup is unsupported for %q", s.mediaType)
		}
		raw, found, err := jsonField(s.raw, name, true)
		if err != nil || !found {
			return nil, found, err
		}
		if err := decoder.Decode(raw, target.Interface()); err != nil {
			return nil, false, fmt.Errorf("decode JSON body field %q: %w", name, err)
		}
		return result(), true, nil
	}
	fieldDecoder, ok := decoder.(FieldDecoder)
	if !ok {
		return nil, false, fmt.Errorf("body decoder for %q does not support named fields", s.mediaType)
	}
	found, err := fieldDecoder.DecodeField(s.raw, name, target.Interface())
	if err != nil || !found {
		return nil, found, err
	}
	return result(), true, nil
}

func decodeTarget(targetType reflect.Type) (reflect.Value, func() interface{}) {
	if targetType.Kind() == reflect.Ptr {
		target := reflect.New(targetType.Elem())
		return target, target.Interface
	}
	target := reflect.New(targetType)
	return target, func() interface{} { return target.Elem().Interface() }
}
