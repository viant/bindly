package body

import (
	"context"
	"errors"
	"github.com/viant/x/shape"
	"reflect"
	"testing"
)

type rootInputRecord struct {
	ID     *int                       `json:"id"`
	Secret string                     `internal:"true"`
	Has    *struct{ ID, Secret bool } `json:"-" setMarker:"true"`
}
type rootInputLocator interface {
	ValueWithBodyInputTag(context.Context, reflect.Type, string, reflect.StructTag) (any, bool, error)
}

func rootInputValue(t *testing.T, s *Source, ctx context.Context, typ reflect.Type, name string) (any, bool, error) {
	t.Helper()
	l, ok := any(s).(rootInputLocator)
	if !ok {
		t.Fatal("native root input policy API missing")
	}
	return l.ValueWithBodyInputTag(ctx, typ, name, `jsonx:"objectToSlice"`)
}
func TestRootInputTagRepresentationAndDecoderOwnership(t *testing.T) {
	target := reflect.TypeFor[[]rootInputRecord]()
	plan, err := shape.CompileRewrite(target)
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := plan.RewriteRepresentation(func(_ string, _ reflect.Type, f *shape.FieldMetadata) (*shape.FieldMetadata, error) {
		if f.Name == "ID" {
			copy := *f
			copy.Tag = `json:"wireId"`
			return &copy, nil
		}
		return f, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, registered := range []bool{false, true} {
		options := []Option{WithRepresentation(metadata)}
		if registered {
			options = append(options, WithDecoders(NewDecoderRegistry()))
		}
		source, _ := New([]byte(`{"wireId":1,"Secret":"forged"}`), "application/json", nil, options...)
		result, found, err := rootInputValue(t, source, context.Background(), target, "")
		if err != nil || !found {
			t.Fatalf("result %v found %v err %v", result, found, err)
		}
		rows := result.([]rootInputRecord)
		if len(rows) != 1 || rows[0].ID == nil || *rows[0].ID != 1 || rows[0].Secret != "" || rows[0].Has == nil || !rows[0].Has.ID {
			t.Fatalf("metadata not preserved %+v", rows)
		}
	}
	registry := NewDecoderRegistry()
	called := false
	registry.Register("application/json", DecoderFunc(func([]byte, any) error { called = true; return nil }))
	source, _ := New([]byte(`{}`), "application/json", nil, WithDecoders(registry))
	if _, _, err := rootInputValue(t, source, context.Background(), target, ""); err == nil || called {
		t.Fatal("overrode configured decoder")
	}
}
func TestRootInputTagDeferredAndInvalidSources(t *testing.T) {
	target := reflect.TypeFor[[]rootInputRecord]()
	cause := errors.New("load failed")
	failed := NewDeferred(func(context.Context) (*Source, error) { return nil, cause })
	if _, _, err := rootInputValue(t, failed, context.Background(), target, ""); !errors.Is(err, cause) {
		t.Fatalf("lost load error %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cancelled := NewDeferred(func(context.Context) (*Source, error) { t.Fatal("loaded cancelled source"); return nil, nil })
	if _, _, err := rootInputValue(t, cancelled, ctx, target, ""); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	for _, media := range []string{"text/plain", "multipart/form-data"} {
		source, _ := New([]byte(`{}`), media, nil)
		if _, _, err := rootInputValue(t, source, context.Background(), target, ""); err == nil {
			t.Fatalf("accepted %s", media)
		}
	}
	source, _ := New([]byte(`{"record":{}}`), "application/json", map[string]string{"": "record"})
	if _, _, err := rootInputValue(t, source, context.Background(), target, ""); err == nil {
		t.Fatal("accepted aliased selected member")
	}
}
