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
	structjson "github.com/viant/structology/encoding/json"
	xshape "github.com/viant/x/shape"
)

type Source struct {
	representation *xshape.Representation
	deferred       *deferredSource
	raw            []byte
	mediaType      string
	exact          bool
	aliases        map[string]string
	decoders       *DecoderRegistry
	unknownFields  structjson.UnknownFieldPolicy
	multipart      *multipart.Form
}
type Option func(*Source)

// WithRepresentation opts into immutable field metadata over the linked body.
func WithRepresentation(metadata *xshape.Representation) Option {
	return func(s *Source) {
		if metadata != nil && metadata.Changed() {
			s.representation = metadata
		}
	}
}

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
	if s.unknownFields != structjson.IgnoreUnknown && s.unknownFields != structjson.ErrorOnUnknown {
		return nil, &input.Error{Cause: fmt.Errorf("unsupported body unknown-field policy %d", s.unknownFields)}
	}
	if s.unknownFields == structjson.ErrorOnUnknown {
		if mediaType != "" && mediaType != "application/json" && !strings.HasSuffix(mediaType, "+json") {
			return nil, &input.Error{Cause: fmt.Errorf("body unknown-field policy requires JSON media")}
		}
		if _, configured := s.decoders.Lookup(mediaType); configured {
			return nil, &input.Error{Cause: fmt.Errorf("body unknown-field policy requires the native JSON decoder")}
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
	return s.valueWithPolicies(ctx, target, name, policy, "", false)
}

// ValueWithBodyInputTag applies an authored whole-body policy per lookup. The
// shared source and deferred request bytes remain unchanged for strict bindings.
func (s *Source) ValueWithBodyInputTag(ctx context.Context, target reflect.Type, name string, tag reflect.StructTag) (any, bool, error) {
	return s.valueWithPolicies(ctx, target, name, "", tag, false)
}

// ValueWithBodyRepresentation keeps the existing source and decoder policies,
// applying this binding's immutable metadata to a detached source view. Configured
// decoders must accept representation metadata; unsupported media fail explicitly.
func (s *Source) ValueWithBodyRepresentation(ctx context.Context, target reflect.Type, name string, metadata *xshape.Representation, policy string, tag reflect.StructTag) (any, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	if s != nil && s.deferred != nil {
		resolved, err := s.resolve(ctx)
		if err != nil {
			return nil, false, err
		}
		return resolved.ValueWithBodyRepresentation(ctx, target, name, metadata, policy, tag)
	}
	if s == nil {
		return nil, false, nil
	}
	if metadata == nil || !metadata.Changed() {
		return s.valueWithPolicies(ctx, target, name, policy, tag, false)
	}
	copy := *s
	copy.representation = metadata
	return copy.valueWithPolicies(ctx, target, name, policy, tag, true)
}

func (s *Source) valueWithPolicies(ctx context.Context, target reflect.Type, name, policy string, tag reflect.StructTag, bindingRepresentation bool) (any, bool, error) {
	if tag != "" {
		if name != "" {
			return nil, false, &input.Error{Cause: fmt.Errorf("body input tag requires whole body")}
		}
		if err := bodyjson.ValidateRootInputTag(tag, target); err != nil {
			return nil, false, &input.Error{Cause: err}
		}
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}
	}

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
		return resolved.valueWithPolicies(ctx, target, name, policy, tag, bindingRepresentation)
	}
	if s == nil {
		return nil, false, nil
	}
	if s.representation != nil && target != nil && target != reflect.TypeFor[json.RawMessage]() && s.representation.Original() != target {
		return nil, false, &input.Error{Cause: fmt.Errorf("body representation expects %s, got %s", s.representation.Original(), target)}
	}
	raw := s.raw
	if alias, ok := s.aliases[name]; ok {
		name = alias
	}
	if policy != "" && name != "" {
		return nil, false, &input.Error{Cause: fmt.Errorf("bodyNullPolicy cannot select an aliased named body field")}
	}
	// A registry may change after New. Admission and dispatch must use the same
	// resolved decoder so late registration cannot override native source policy.
	decoder, configured := s.decoders.Lookup(s.mediaType)
	if s.unknownFields == structjson.ErrorOnUnknown && configured {
		return nil, false, &input.Error{Cause: fmt.Errorf("body unknown-field policy requires the native JSON decoder")}
	}
	metadataDecoder, supportsRepresentation := decoder.(interface {
		UnmarshalWithRepresentation([]byte, any, *xshape.Representation) error
	})
	jsonMedia := s.mediaType == "" || s.mediaType == "application/json" || strings.HasSuffix(s.mediaType, "+json")
	if bindingRepresentation {
		if strings.HasPrefix(s.mediaType, "multipart/") || (!jsonMedia && (!configured || name != "")) {
			return nil, false, &input.Error{Cause: fmt.Errorf("body decoder for %q does not support representation metadata for this binding", s.mediaType)}
		}
		if configured && !supportsRepresentation {
			return nil, false, &input.Error{Cause: fmt.Errorf("configured body decoder does not accept representation metadata")}
		}
	}
	if tag != "" {
		if name != "" {
			return nil, false, &input.Error{Cause: fmt.Errorf("body input tag cannot select an aliased member")}
		}
		if !jsonMedia {
			return nil, false, &input.Error{Cause: fmt.Errorf("body input tag requires JSON media")}
		}
		if configured {
			if _, native := decoder.(jsonDecoder); !native {
				return nil, false, &input.Error{Cause: fmt.Errorf("body input tag cannot override configured decoder")}
			}
		}
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
	if jsonMedia {
		if name != "" {
			var found bool
			var err error
			raw, found, err = s.selectJSON(name)
			if err != nil {
				return nil, false, &input.Error{Cause: err}
			}
			if !found {
				return nil, false, nil
			}
		}

		if tag != "" {
			converted, omit, err := bodyjson.ApplyRootInputTag(raw, tag, target)
			if err != nil {
				return nil, true, &input.Error{Cause: err}
			}
			if omit {
				return nil, false, nil
			}
			raw = converted
		}
		if bindingRepresentation {
			if err := ValidateRequired(raw, target, s.representation); err != nil {
				return nil, true, err
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
		if !configured {
			value, err := bodyjson.DecodePublicWithUnknownFieldPolicy(raw, target, s.unknownFields, s.representation)
			return value, true, err
		}
		if !bindingRepresentation {
			filtered, err := bodyjson.FilterPublic(raw, target, s.representation)
			if err != nil {
				return nil, true, &input.Error{Cause: err}
			}
			raw = filtered
		}
		value := reflect.New(target)
		decode := decoder.Decode
		if s.representation != nil {
			if !supportsRepresentation {
				return nil, true, &input.Error{Cause: fmt.Errorf("configured body decoder does not accept representation metadata")}
			}
			decode = func(raw []byte, destination any) error {
				return metadataDecoder.UnmarshalWithRepresentation(raw, destination, s.representation)
			}
		}
		if err := decode(raw, value.Interface()); err != nil {
			return nil, true, &input.Error{Cause: err}
		}
		if err := s.markPresence(value.Elem(), raw); err != nil {
			return nil, true, err
		}
		return value.Elem().Interface(), true, nil
	}
	if configured {
		if target == nil {
			return nil, false, &input.Error{Cause: fmt.Errorf("body binding source type is required")}
		}
		value := reflect.New(target)
		if name == "" {
			decode := decoder.Decode
			if bindingRepresentation {
				decode = func(raw []byte, destination any) error {
					return metadataDecoder.UnmarshalWithRepresentation(raw, destination, s.representation)
				}
			}
			if err := decode(raw, value.Interface()); err != nil {
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
