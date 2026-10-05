package body

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

// Decoder unmarshals a body value into a typed target.
type Decoder interface{ Decode([]byte, any) error }
type DecoderFunc func([]byte, any) error

func (f DecoderFunc) Decode(raw []byte, target any) error { return f(raw, target) }

type FieldDecoder interface {
	Decoder
	DecodeField([]byte, string, any) (bool, error)
}
type DecoderRegistry struct {
	mu      sync.RWMutex
	byMedia map[string]Decoder
}

func NewDecoderRegistry() *DecoderRegistry {
	r := &DecoderRegistry{byMedia: map[string]Decoder{}}
	r.Register("application/json", jsonDecoder{})
	return r
}
func (r *DecoderRegistry) Register(mediaType string, decoder Decoder) {
	if r == nil || decoder == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.byMedia == nil {
		r.byMedia = map[string]Decoder{}
	}
	r.byMedia[strings.ToLower(strings.TrimSpace(mediaType))] = decoder
}
func (r *DecoderRegistry) Lookup(mediaType string) (Decoder, bool) {
	if r == nil {
		return nil, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	decoder, ok := r.byMedia[strings.ToLower(strings.TrimSpace(mediaType))]
	return decoder, ok
}
func WithDecoders(decoders *DecoderRegistry) Option { return func(s *Source) { s.decoders = decoders } }

type jsonDecoder struct{}

func (jsonDecoder) Decode(raw []byte, target any) error { return json.Unmarshal(raw, target) }
func (j jsonDecoder) DecodeField(raw []byte, name string, target any) (bool, error) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		return false, err
	}
	selected, found := object[name]
	if !found {
		for key, candidate := range object {
			if strings.EqualFold(key, name) {
				if found {
					return false, fmt.Errorf("ambiguous body field %q", name)
				}
				selected, found = candidate, true
			}
		}
	}
	if !found {
		return false, nil
	}
	return true, j.Decode(selected, target)
}
