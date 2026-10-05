package conv

import (
	"context"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/viant/tagly/tags"
)

// TestIntTransformer_Transform verifies value conversion behaviour.
func TestIntTransformer_Transform(t *testing.T) {
	type testCase struct {
		name      string
		input     interface{}
		want      interface{}
		wantError bool
	}

	testCases := []testCase{
		{name: "nil input returns zero", input: nil, want: 0},
		{name: "int passthrough", input: int(7), want: int(7)},
		{name: "int32 conversion", input: int32(8), want: int(8)},
		{name: "int64 conversion", input: int64(9), want: int(9)},
		{name: "float32 conversion truncates", input: float32(3.7), want: int(3)},
		{name: "float64 conversion truncates", input: float64(4.9), want: int(4)},
		{name: "string decimal", input: "42", want: int(42)},
		{name: "string invalid", input: "forty-two", wantError: true},
		{name: "unsupported type", input: []byte("10"), wantError: true},
	}

	ctx := context.Background()
	var cfg tags.Values
	tr, err := NewIntTransformer(ctx, cfg, reflect.TypeOf(int(0)), nil)
	assert.NoError(t, err)

	for _, tc := range testCases {
		got, err := tr.Transform(ctx, nil, tc.input)
		if tc.wantError {
			assert.Error(t, err, tc.name)
			continue
		}

		assert.NoError(t, err, tc.name)
		assert.EqualValues(t, tc.want, got, tc.name)
	}
}

// TestNewIntTransformer_DestTypeValidation ensures only int kinds are accepted.
func TestNewIntTransformer_DestTypeValidation(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name      string
		dest      reflect.Type
		wantError bool
	}{
		{"int ok", reflect.TypeOf(int(0)), false},
		{"int32 ok", reflect.TypeOf(int32(0)), false},
		{"int64 ok", reflect.TypeOf(int64(0)), false},
		{"string not allowed", reflect.TypeOf(""), true},
		{"bool not allowed", reflect.TypeOf(true), true},
	}

	for _, tc := range tests {
		var cfg tags.Values
		tr, err := NewIntTransformer(ctx, cfg, tc.dest, nil)
		if tc.wantError {
			assert.Error(t, err, tc.name)
			assert.Nil(t, tr, tc.name)
			continue
		}
		assert.NoError(t, err, tc.name)
		assert.NotNil(t, tr, tc.name)
	}
}
