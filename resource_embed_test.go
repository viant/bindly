package bindly

import (
	"embed"
	"io"
	"strings"
	"testing"
)

//go:embed testdata/query.sql
var testResources embed.FS

func TestGoEmbedResourceAvailableThroughInjector(t *testing.T) {
	injector, err := NewInjector(
		WithResourceFS("assets", testResources),
	)
	if err != nil {
		t.Fatalf("NewInjector() error = %v", err)
	}
	data, err := injector.ReadResource("assets:testdata/query.sql")
	if err != nil {
		t.Fatalf("ReadResource() error = %v", err)
	}
	if actual := strings.TrimSpace(string(data)); actual != "SELECT id, name FROM users" {
		t.Fatalf("SQL = %q", actual)
	}
	registered, ok := injector.ResourceFS("assets")
	if !ok {
		t.Fatal("ResourceFS(assets) was not found")
	}
	if _, ok := registered.(embed.FS); !ok {
		t.Fatalf("ResourceFS(assets) type = %T, want embed.FS", registered)
	}
	file, err := registered.Open("testdata/query.sql")
	if err != nil {
		t.Fatalf("embedded Open() error = %v", err)
	}
	t.Cleanup(func() { _ = file.Close() })
	opened, err := io.ReadAll(file)
	if err != nil || strings.TrimSpace(string(opened)) != "SELECT id, name FROM users" {
		t.Fatalf("embedded read = %q, err=%v", opened, err)
	}
}
