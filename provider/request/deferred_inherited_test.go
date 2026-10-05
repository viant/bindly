package request

import (
	"context"
	"github.com/viant/bindly"
	"net/http"
	"testing"
)

func TestDeferredRequestInheritedChildDemand(t *testing.T) {
	reader := &demandReader{payload: `{"id":9007199254740993,"enabled":false}`}
	req, _ := http.NewRequest("POST", "http://example.invalid", reader)
	req.Header.Set("Content-Type", "application/json")
	scope, err := NewDeferred(req)
	if err != nil {
		t.Fatal(err)
	}
	defer scope.Close()
	parent, err := bindly.NewInjector(bindly.WithProviders(scope.Providers()...))
	if err != nil {
		t.Fatal(err)
	}
	child, err := parent.ForScope()
	if err != nil {
		t.Fatal(err)
	}
	metadata := struct {
		Method string `bind:"kind=http_request,in=method"`
	}{}
	if err = parent.Bind(context.Background(), &metadata); err != nil {
		t.Fatal(err)
	}
	if reader.reads != 0 || metadata.Method != "POST" {
		t.Fatal("metadata initialized body")
	}
	for i := 0; i < 2; i++ {
		input := struct {
			ID      int64 `bind:"kind=body,in=id"`
			Enabled *bool `bind:"kind=body,in=enabled"`
		}{}
		if err = child.Bind(context.Background(), &input); err != nil {
			t.Fatal(err)
		}
		if input.ID != 9007199254740993 || input.Enabled == nil || *input.Enabled {
			t.Fatalf("lost inherited precision/presence: %+v", input)
		}
	}
	if reader.bytes != len(`{"id":9007199254740993,"enabled":false}`) {
		t.Fatalf("snapshot read more than once: %d", reader.bytes)
	}
}
