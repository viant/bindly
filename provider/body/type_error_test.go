package body_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/viant/bindly/provider/body"
)

func TestCustomDecoderTypeErrorUnchanged(t *testing.T) {
	original := &json.UnmarshalTypeError{Value: "string", Type: reflect.TypeFor[int](), Offset: 8, Struct: "Owned", Field: "value"}
	saved := *original
	registry := body.NewDecoderRegistry()
	registry.Register("application/json", body.DecoderFunc(func([]byte, any) error { return original }))
	source, err := body.New([]byte(`[{"value":"bad"}]`), "application/json", nil, body.WithDecoders(registry))
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = source.Value(context.Background(), reflect.TypeFor[[]struct {
		Value int `json:"value"`
	}](), "")
	var got *json.UnmarshalTypeError
	if !errors.As(err, &got) || got != original || *got != saved {
		t.Fatal("external decoder diagnostic changed")
	}
}
