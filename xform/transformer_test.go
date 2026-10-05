package xform

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/viant/bindly/locator"
)

// dummyTransformer is a simple implementation of Transformer used only for tests.
type dummyTransformer struct{}

func (d *dummyTransformer) Transform(_ context.Context, _ locator.Resolver, input interface{}) (interface{}, error) {
	// Echo the input back so we can verify the interface contract
	return input, nil
}

// TestTransformerInterfaceContract validates that a type implementing
// Transform(ctx, resolver, input) (interface{}, error) satisfies the
// Transformer interface defined in transformer.go.
func TestTransformerInterfaceContract(t *testing.T) {
	var _ Transformer = (*dummyTransformer)(nil)

	ctx := context.Background()
	tr := &dummyTransformer{}

	input := "test-value"
	got, err := tr.Transform(ctx, nil, input)

	assert.NoError(t, err)
	assert.Equal(t, input, got)
}

