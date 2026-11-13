package body

import (
	"context"
	"encoding/json"
	"errors"
	"mime/multipart"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/viant/bindly/input"
	structjson "github.com/viant/structology/encoding/json"
	"github.com/viant/x/shape"
)

type capabilityMetadataTarget struct {
	Public string                         `json:"public"`
	Secret string                         `json:"secret"`
	Has    *struct{ Public, Secret bool } `json:"-" setMarker:"true"`
}

func capabilityMetadata(t *testing.T) *shape.Representation {
	t.Helper()
	walker, err := shape.CompileRewrite(reflect.TypeFor[*capabilityMetadataTarget]())
	require.NoError(t, err)
	metadata, err := walker.RewriteRepresentation(func(_ string, _ reflect.Type, field *shape.FieldMetadata) (*shape.FieldMetadata, error) {
		if field.Name == "Secret" {
			return &shape.FieldMetadata{Name: field.Name, Tag: `json:"-" internal:"true"`}, nil
		}
		return nil, nil
	})
	require.NoError(t, err)
	return metadata
}

type capabilityDecoder struct {
	calls     int
	raw       []byte
	metadata  *shape.Representation
	err       error
	unmarshal func([]byte, any, *shape.Representation) error
}

func (d *capabilityDecoder) Decode([]byte, any) error { panic("representation owner bypassed") }
func (d *capabilityDecoder) DecodeField([]byte, string, any) (bool, error) {
	panic("named representation owner bypassed")
}
func (d *capabilityDecoder) UnmarshalWithRepresentation(raw []byte, target any, metadata *shape.Representation) error {
	d.calls++
	d.raw = append([]byte(nil), raw...)
	d.metadata = metadata
	if d.err != nil {
		return d.err
	}
	if d.unmarshal != nil {
		return d.unmarshal(raw, target, metadata)
	}
	return structjson.UnmarshalStandard(raw, target, structjson.WithRepresentation(metadata), structjson.WithPresenceTracking(true))
}

func TestBindingRepresentationRejectsUnsupportedDecoderCapabilities(t *testing.T) {
	metadata := capabilityMetadata(t)
	raw := []byte(`{"public":"ok","secret":"injected"}`)
	for _, media := range []string{"application/json", "application/custom"} {
		for _, name := range []string{"", "payload"} {
			t.Run(media+"/"+name, func(t *testing.T) {
				calls := 0
				registry := &DecoderRegistry{}
				registry.Register(media, DecoderFunc(func(raw []byte, target any) error { calls++; return json.Unmarshal(raw, target) }))
				source, err := New(raw, media, nil, WithDecoders(registry))
				require.NoError(t, err)
				_, _, err = source.ValueWithBodyRepresentation(t.Context(), metadata.Original(), name, metadata, "", "")
				var invalid *input.Error
				require.ErrorAs(t, err, &invalid)
				require.ErrorContains(t, invalid.Cause, "representation metadata")
				require.Zero(t, calls)
			})
		}
	}
	for _, media := range []string{"application/custom", "multipart/form-data"} {
		source, err := New(raw, media, nil, WithMultipartForm(&multipart.Form{Value: map[string][]string{"payload": {"injected"}}}))
		require.NoError(t, err)
		_, _, err = source.ValueWithBodyRepresentation(t.Context(), metadata.Original(), "payload", metadata, "", "")
		var invalid *input.Error
		require.ErrorAs(t, err, &invalid)
		require.ErrorContains(t, invalid.Cause, "representation metadata")
	}
	for _, raw := range []string{"", "null"} {
		registry := &DecoderRegistry{}
		registry.Register("application/json", DecoderFunc(json.Unmarshal))
		source, err := New([]byte(raw), "application/json", nil, WithDecoders(registry))
		require.NoError(t, err)
		_, _, err = source.ValueWithBodyRepresentation(t.Context(), metadata.Original(), "", metadata, "", "")
		var invalid *input.Error
		require.ErrorAs(t, err, &invalid, "empty/null cannot bypass decoder capability checks")
	}
	decoder := &capabilityDecoder{}
	registry := &DecoderRegistry{}
	registry.Register("application/custom", decoder)
	source, err := New(raw, "application/custom", nil, WithDecoders(registry))
	require.NoError(t, err)
	_, _, err = source.ValueWithBodyRepresentation(t.Context(), metadata.Original(), "payload", metadata, "", "")
	var invalid *input.Error
	require.ErrorAs(t, err, &invalid)
	require.ErrorContains(t, invalid.Cause, "representation metadata")
	require.Zero(t, decoder.calls)
}

func TestBindingRepresentationPreservesConfiguredOwnerAndOriginalBytes(t *testing.T) {
	metadata := capabilityMetadata(t)
	raw := []byte("{ \"public\":\"ok\", \"secret\":\"injected\", \"unknown\":true }")
	for _, media := range []string{"application/json", "application/custom"} {
		t.Run(media, func(t *testing.T) {
			decoder := &capabilityDecoder{}
			registry := &DecoderRegistry{}
			registry.Register(media, decoder)
			source, err := New(raw, media, nil, WithDecoders(registry))
			require.NoError(t, err)
			value, found, err := source.ValueWithBodyRepresentation(t.Context(), metadata.Original(), "", metadata, "", "")
			require.NoError(t, err)
			require.True(t, found)
			require.Equal(t, 1, decoder.calls)
			require.Equal(t, raw, decoder.raw)
			require.Same(t, metadata, decoder.metadata)
			actual := value.(*capabilityMetadataTarget)
			require.Equal(t, "ok", actual.Public)
			require.Empty(t, actual.Secret)
			require.NotNil(t, actual.Has)
			require.True(t, actual.Has.Public)
			require.False(t, actual.Has.Secret)
		})
	}
}

func TestBindingRepresentationNilMetadataRetainsCustomAndLegacySourceBehavior(t *testing.T) {
	metadata := capabilityMetadata(t)
	for _, legacy := range []bool{false, true} {
		calls := 0
		registry := &DecoderRegistry{}
		registry.Register("application/custom", DecoderFunc(func(raw []byte, target any) error { calls++; return json.Unmarshal(raw, target) }))
		options := []Option{WithDecoders(registry)}
		if legacy {
			options = append(options, WithRepresentation(metadata))
		}
		source, err := New([]byte(`{"public":"ok","secret":"legacy"}`), "application/custom", nil, options...)
		require.NoError(t, err)
		value, found, err := source.ValueWithBodyRepresentation(t.Context(), metadata.Original(), "", nil, "", "")
		require.NoError(t, err)
		require.True(t, found)
		require.Equal(t, "legacy", value.(*capabilityMetadataTarget).Secret)
		require.Equal(t, 1, calls)
	}
}

func TestBindingRepresentationDoesNotParseConfiguredForeignBytes(t *testing.T) {
	metadata := capabilityMetadata(t)
	raw := []byte("public=ok\x00secret=injected\n")
	decoder := &capabilityDecoder{unmarshal: func(_ []byte, target any, _ *shape.Representation) error {
		*target.(**capabilityMetadataTarget) = &capabilityMetadataTarget{Public: "ok"}
		return nil
	}}
	registry := &DecoderRegistry{}
	registry.Register("application/custom", decoder)
	source, err := New(raw, "application/custom", nil, WithDecoders(registry))
	require.NoError(t, err)
	value, found, err := source.ValueWithBodyRepresentation(t.Context(), metadata.Original(), "", metadata, "", "")
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, "ok", value.(*capabilityMetadataTarget).Public)
	require.Equal(t, raw, decoder.raw)
	require.Same(t, metadata, decoder.metadata)
	require.Equal(t, 1, decoder.calls)
}

func TestBindingRepresentationPreservesOwnerFailuresAndCancellation(t *testing.T) {
	metadata := capabilityMetadata(t)
	unavailable := errors.New("decoder unavailable")
	for _, cause := range []error{unavailable, context.Canceled, context.DeadlineExceeded} {
		decoder := &capabilityDecoder{err: cause}
		registry := &DecoderRegistry{}
		registry.Register("application/custom", decoder)
		source, err := New([]byte(`{"public":"ok"}`), "application/custom", nil, WithDecoders(registry))
		require.NoError(t, err)
		_, _, err = source.ValueWithBodyRepresentation(t.Context(), metadata.Original(), "", metadata, "", "")
		require.ErrorIs(t, err, cause)
		require.Equal(t, 1, decoder.calls)
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		_, _, err = source.ValueWithBodyRepresentation(ctx, metadata.Original(), "", metadata, "", "")
		require.ErrorIs(t, err, context.Canceled)
		require.Equal(t, 1, decoder.calls)
	}
}

func TestUnknownFieldPolicyRejectsLateRegisteredDecoder(t *testing.T) {
	registry := &DecoderRegistry{}
	source, err := New([]byte(`{"public":"ok","unexpected":true}`), "application/json", nil, WithDecoders(registry), WithUnknownFieldPolicy(structjson.ErrorOnUnknown))
	require.NoError(t, err)
	_, _, err = source.Value(t.Context(), reflect.TypeFor[*capabilityMetadataTarget](), "")
	var invalid *input.Error
	require.ErrorAs(t, err, &invalid)
	require.ErrorContains(t, invalid.Cause, `unknown field "unexpected"`)
	calls := 0
	registry.Register("application/json", DecoderFunc(func(raw []byte, target any) error { calls++; return json.Unmarshal(raw, target) }))
	_, _, err = source.Value(t.Context(), reflect.TypeFor[*capabilityMetadataTarget](), "")
	require.ErrorAs(t, err, &invalid)
	require.ErrorContains(t, invalid.Cause, "requires the native JSON decoder")
	require.Zero(t, calls)
}

func TestUnknownFieldPolicyConcurrentRegistrationCannotChangeDispatchOwner(t *testing.T) {
	registry := &DecoderRegistry{}
	source, err := New([]byte(`{"unexpected":true}`), "application/json", nil, WithDecoders(registry), WithUnknownFieldPolicy(structjson.ErrorOnUnknown))
	require.NoError(t, err)
	var calls atomic.Int32
	decoder := DecoderFunc(func(raw []byte, target any) error { calls.Add(1); return json.Unmarshal(raw, target) })
	var workers sync.WaitGroup
	for i := 0; i < 16; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for j := 0; j < 16; j++ {
				_, _, err := source.Value(t.Context(), reflect.TypeFor[*capabilityMetadataTarget](), "")
				if err == nil {
					t.Error("strict decoding accepted unknown field")
				}
			}
		}()
	}
	registry.Register("application/json", decoder)
	workers.Wait()
	require.Zero(t, calls.Load())
}
