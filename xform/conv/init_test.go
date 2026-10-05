package conv

import (
	"context"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/viant/bindly/xform"
	"github.com/viant/tagly/tags"
)

// TestInit_RegistersStandardTransformers ensures Init wires factories into the registry.
func TestInit_RegistersStandardTransformers(t *testing.T) {
	registry := xform.NewRegistry()

	Init(registry)

	tests := []struct {
		name string
	}{
		{"string"},
		{"int"},
		{"bool"},
	}

	ctx := context.Background()

	for _, tc := range tests {
		factory, ok := registry.Lookup(tc.name)
		if assert.True(t, ok, tc.name+" factory registered") {
			// Sanity check that the factory can create a transformer for its expected type.
			var dest reflect.Type
			switch tc.name {
			case "string":
				dest = reflect.TypeOf("")
			case "int":
				dest = reflect.TypeOf(int(0))
			case "bool":
				dest = reflect.TypeOf(true)
			}

			var cfg tags.Values
			tr, err := factory.Create(ctx, cfg, dest, nil)
			assert.NoError(t, err, tc.name)
			assert.NotNil(t, tr, tc.name)
		}
	}
}
