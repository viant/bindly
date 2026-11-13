package bindly

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/viant/bindly/input"
	"github.com/viant/bindly/provider/body"
	"github.com/viant/bindly/state"
	"github.com/viant/x/shape"
)

func TestBodyRepresentationRejectsConfiguredForeignDecoderThroughBinding(t *testing.T) {
	type record struct {
		Public string `json:"public"`
		Secret string `json:"secret"`
	}
	type target struct{ Body *record }
	walker, err := shape.CompileRewrite(reflect.TypeFor[*record]())
	require.NoError(t, err)
	metadata, err := walker.RewriteRepresentation(func(_ string, _ reflect.Type, field *shape.FieldMetadata) (*shape.FieldMetadata, error) {
		if field.Name == "Secret" {
			return &shape.FieldMetadata{Name: field.Name, Tag: `json:"-" internal:"true"`}, nil
		}
		return nil, nil
	})
	require.NoError(t, err)
	calls := 0
	registry := body.NewDecoderRegistry()
	registry.Register("application/custom", body.DecoderFunc(func(raw []byte, target any) error { calls++; return json.Unmarshal(raw, target) }))
	source, err := body.New([]byte(`{"public":"ok","secret":"injected"}`), "application/custom", nil, body.WithDecoders(registry))
	require.NoError(t, err)
	injector, err := NewInjector(WithProviders(source))
	require.NoError(t, err)
	plan, err := injector.CompilePlan(reflect.TypeFor[target](), BindingSpec{Path: "Body", Location: state.Location{Kind: "body"}, BodyRepresentation: metadata})
	require.NoError(t, err)
	actual := &target{}
	err = injector.Bind(t.Context(), actual, WithPlan(plan))
	var invalid *input.Error
	require.ErrorAs(t, err, &invalid)
	require.ErrorContains(t, invalid.Cause, "representation metadata")
	require.Nil(t, actual.Body)
	require.Zero(t, calls)
}
