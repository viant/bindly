package conv

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/viant/tagly/tags"
)

type stringerType struct{}

func (s stringerType) String() string { return "stringer-value" }

// TestStringTransformer_Transform verifies conversion for various inputs.
func TestStringTransformer_Transform(t *testing.T) {
	type testCase struct {
		name  string
		input interface{}
		want  string
	}

	testCases := []testCase{
		{name: "nil input returns empty", input: nil, want: ""},
		{name: "string passthrough", input: "text", want: "text"},
		{name: "byte slice", input: []byte("bytes"), want: "bytes"},
		{name: "fmt.Stringer", input: stringerType{}, want: "stringer-value"},
		{name: "fallback fmt.Sprint", input: 123, want: fmt.Sprintf("%v", 123)},
	}

	ctx := context.Background()
	var cfg tags.Values
	tr, err := NewStringTransformer(ctx, cfg, reflect.TypeOf(""), nil)
	assert.NoError(t, err)

	for _, tc := range testCases {
		got, err := tr.Transform(ctx, nil, tc.input)
		assert.NoError(t, err, tc.name)
		assert.EqualValues(t, tc.want, got, tc.name)
	}
}

// TestNewStringTransformer_DestTypeValidation ensures only string kind is accepted.
func TestNewStringTransformer_DestTypeValidation(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name      string
		dest      reflect.Type
		wantError bool
	}{
		{"string ok", reflect.TypeOf(""), false},
		{"int not allowed", reflect.TypeOf(int(0)), true},
		{"bool not allowed", reflect.TypeOf(true), true},
	}

	for _, tc := range tests {
		var cfg tags.Values
		tr, err := NewStringTransformer(ctx, cfg, tc.dest, nil)
		if tc.wantError {
			assert.Error(t, err, tc.name)
			assert.Nil(t, tr, tc.name)
			continue
		}
		assert.NoError(t, err, tc.name)
		assert.NotNil(t, tr, tc.name)
	}
}
