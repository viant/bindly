package body

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/viant/bindly/input"
	structjson "github.com/viant/structology/encoding/json"
)

type unknownPolicyOwned struct{ Raw string }

func (v *unknownPolicyOwned) UnmarshalJSON(raw []byte) error { v.Raw = string(raw); return nil }

func TestUnknownFieldPolicyUsesNativeBodyPipeline(t *testing.T) {
	type child struct {
		Q *string `json:"q" jsonx:"numberToString"`
	}
	type payload struct {
		Q      *string                                                         `json:"q" format:"inputNames=legacy" jsonx:"numberToString"`
		Flag   *int                                                            `json:"flag" jsonx:"boolToInt,null=omit"`
		Bill   *string                                                         `json:"bill" jsonx:"boolIntToYN,null=omit"`
		Plain  int                                                             `json:"plain"`
		Rows   []child                                                         `json:"rows"`
		Secret *int                                                            `json:"secret" internal:"true" jsonx:"boolToInt"`
		Date   time.Time                                                       `json:"date"`
		Owned  unknownPolicyOwned                                              `json:"owned"`
		Has    *struct{ Q, Flag, Bill, Plain, Rows, Secret, Date, Owned bool } `json:"-" setMarker:"true"`
	}
	for _, policy := range []structjson.UnknownFieldPolicy{structjson.IgnoreUnknown, structjson.ErrorOnUnknown} {
		source, err := New([]byte(`{"legacy":50,"flag":null,"bill":"","rows":[{"q":51}],"secret":true,"date":"2026-01-02 03:04:05","owned":{"arbitrary":true}}`), "application/json", nil, WithUnknownFieldPolicy(policy))
		require.NoError(t, err)
		for i := 0; i < 2; i++ {
			value, found, err := source.Value(t.Context(), reflect.TypeFor[payload](), "")
			if err != nil {
				var invalid *input.Error
				if errors.As(err, &invalid) {
					t.Fatalf("decode policy %v: %v", policy, invalid.Cause)
				}
				t.Fatal(err)
			}
			require.True(t, found)
			got := value.(payload)
			require.Equal(t, "50", *got.Q)
			require.True(t, got.Has.Q)
			require.Nil(t, got.Flag)
			require.False(t, got.Has.Flag)
			require.NotNil(t, got.Bill)
			require.Empty(t, *got.Bill)
			require.True(t, got.Has.Bill, "converted empty string remains present")
			require.Equal(t, "51", *got.Rows[0].Q)
			require.Nil(t, got.Secret)
			require.False(t, got.Has.Secret)
			require.Equal(t, 2026, got.Date.Year())
			require.Equal(t, `{"arbitrary":true}`, got.Owned.Raw)
		}
		for _, tc := range []struct {
			raw      string
			rejected bool
		}{
			{`{"rows":[{"q":51,"unexpected":true}]}`, policy == structjson.ErrorOnUnknown},
			{`{"plain":true}`, true},
			{`{"Has":{"Secret":true}}`, policy == structjson.ErrorOnUnknown},
			{`{"q":true}`, true},
		} {
			source, err := New([]byte(tc.raw), "application/json", nil, WithUnknownFieldPolicy(policy))
			require.NoError(t, err)
			_, _, err = source.Value(t.Context(), reflect.TypeFor[payload](), "")
			if !tc.rejected {
				require.NoError(t, err)
				continue
			}
			var invalid *input.Error
			require.ErrorAs(t, err, &invalid)
			if tc.raw == `{"plain":true}` {
				var cause *json.UnmarshalTypeError
				require.ErrorAs(t, err, &cause)
			}
			if tc.raw == `{"rows":[{"q":51,"unexpected":true}]}` {
				require.ErrorContains(t, invalid.Cause, `unknown field "unexpected"`)
			}
		}
	}
}

func TestUnknownFieldPolicyRejectsUnsupportedOwnership(t *testing.T) {
	registry := NewDecoderRegistry()
	for _, options := range [][]Option{
		{WithUnknownFieldPolicy(structjson.ErrorOnUnknown), WithDecoders(registry)},
		{WithDecoders(registry), WithUnknownFieldPolicy(structjson.ErrorOnUnknown)},
		{WithUnknownFieldPolicy(structjson.UnknownFieldPolicy(99))},
	} {
		_, err := New([]byte(`{}`), "application/json", nil, options...)
		var invalid *input.Error
		require.ErrorAs(t, err, &invalid)
	}
	_, err := New([]byte(`{}`), "text/plain", nil, WithUnknownFieldPolicy(structjson.ErrorOnUnknown))
	var invalid *input.Error
	require.ErrorAs(t, err, &invalid)
	require.ErrorContains(t, invalid.Cause, "requires JSON media")
	// Existing configured decoders continue owning their contract by default.
	_, err = New([]byte(`{}`), "application/json", nil, WithDecoders(registry))
	require.NoError(t, err)
}

func TestUnknownFieldPolicyPreservesDeferredCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	source := NewDeferred(func(context.Context) (*Source, error) { t.Fatal("cancelled load ran"); return nil, nil })
	_, _, err := source.Value(ctx, reflect.TypeFor[struct{}](), "")
	require.True(t, errors.Is(err, context.Canceled))
}
