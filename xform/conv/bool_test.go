package conv

import (
	"context"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/viant/tagly/tags"
)

// TestBoolTransformer_Transform covers the supported input kinds and error cases.
func TestBoolTransformer_Transform(t *testing.T) {
	ctx := context.Background()
	tr := &BoolTransformer{}

	type testCase struct {
		name      string
		input     interface{}
		want      bool
		wantError bool
	}

	testCases := []testCase{
		{name: "nil input returns false", input: nil, want: false},
		{name: "bool true passthrough", input: true, want: true},
		{name: "bool false passthrough", input: false, want: false},
		{name: "string true", input: "true", want: true},
		{name: "string yes", input: "yes", want: true},
		{name: "string 1", input: "1", want: true},
		{name: "string on", input: "on", want: true},
		{name: "string false", input: "false", want: false},
		{name: "string no", input: "no", want: false},
		{name: "string 0", input: "0", want: false},
		{name: "string off", input: "off", want: false},
		{name: "string empty", input: "", want: false},
		{name: "string invalid", input: "maybe", wantError: true},
		{name: "int zero", input: int(0), want: false},
		{name: "int non-zero", input: int(10), want: true},
		{name: "int32 non-zero", input: int32(1), want: true},
		{name: "int64 zero", input: int64(0), want: false},
		{name: "unsupported type", input: float64(1), wantError: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := tr.Transform(ctx, nil, tc.input)
			if tc.wantError {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			b, ok := out.(bool)
			if !ok {
				t.Fatalf("expected bool result, got %T", out)
			}
			assert.EqualValues(t, tc.want, b)
		})
	}
}

// TestNewBoolTransformer_DestTypeValidation checks destination type validation.
func TestNewBoolTransformer_DestTypeValidation(t *testing.T) {
	ctx := context.Background()
	var config tags.Values

	tests := []struct {
		name      string
		dest      reflect.Type
		wantError bool
	}{
		{"bool ok", reflect.TypeOf(true), false},
		{"int not allowed", reflect.TypeOf(int(0)), true},
		{"string not allowed", reflect.TypeOf(""), true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tr, err := NewBoolTransformer(ctx, config, tc.dest, nil)
			if tc.wantError {
				assert.Error(t, err)
				assert.Nil(t, tr)
				return
			}
			assert.NoError(t, err)
			assert.NotNil(t, tr)
		})
	}
}
