// Package body provides protocol-neutral typed body decoding for Bindly body
// providers. Protocol adapters own transport acquisition and media types.
package body

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync"
)

// Decoder unmarshals one body value into a caller-provided typed target.
type Decoder interface {
	Decode(data []byte, target interface{}) error
}

type DecoderFunc func(data []byte, target interface{}) error

func (f DecoderFunc) Decode(data []byte, target interface{}) error {
	return f(data, target)
}

// FieldDecoder optionally supports resolving one named body field.
type FieldDecoder interface {
	Decoder
	DecodeField(data []byte, name string, target interface{}) (bool, error)
}

// DecoderRegistry owns media-type decoders shared by protocol adapters.
type DecoderRegistry struct {
	mu      sync.RWMutex
	byMedia map[string]Decoder
}

func NewDecoderRegistry() *DecoderRegistry {
	registry := &DecoderRegistry{byMedia: map[string]Decoder{}}
	registry.Register("application/json", jsonDecoder{})
	return registry
}

func (r *DecoderRegistry) Register(mediaType string, decoder Decoder) {
	if r == nil || decoder == nil {
		return
	}
	r.mu.Lock()
	r.byMedia[strings.ToLower(strings.TrimSpace(mediaType))] = decoder
	r.mu.Unlock()
}

func (r *DecoderRegistry) Lookup(mediaType string) (Decoder, bool) {
	if r == nil {
		return nil, false
	}
	r.mu.RLock()
	decoder, ok := r.byMedia[strings.ToLower(strings.TrimSpace(mediaType))]
	r.mu.RUnlock()
	return decoder, ok
}

type jsonDecoder struct{}

func (jsonDecoder) Decode(data []byte, target interface{}) error {
	if err := json.Unmarshal(data, target); err != nil {
		return err
	}
	applyJSONPresence(data, reflect.ValueOf(target))
	return nil
}

func (j jsonDecoder) DecodeField(data []byte, name string, target interface{}) (bool, error) {
	raw, found, err := jsonField(data, name, false)
	if err != nil || !found {
		return found, err
	}
	if err := j.Decode(raw, target); err != nil {
		return false, fmt.Errorf("decode JSON body field %q: %w", name, err)
	}
	return true, nil
}

func jsonField(data []byte, name string, exact bool) ([]byte, bool, error) {
	fields := map[string]json.RawMessage{}
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, false, err
	}
	raw, ok := fields[name]
	if !ok && !exact {
		for candidate, value := range fields {
			if strings.EqualFold(candidate, name) {
				raw, ok = value, true
				break
			}
		}
	}
	return raw, ok, nil
}
