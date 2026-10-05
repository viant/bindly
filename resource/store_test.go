package resource

import (
	"io"
	"testing"
	"testing/fstest"
)

func TestStoreReadsDefaultAndNamedNamespaces(t *testing.T) {
	store := New()
	defaultFS := fstest.MapFS{"default.txt": {Data: []byte("default")}}
	namedFS := fstest.MapFS{"named.txt": {Data: []byte("named")}}
	if err := store.Register("", defaultFS); err != nil {
		t.Fatalf("Register(default) error = %v", err)
	}
	if err := store.Register("assets", namedFS); err != nil {
		t.Fatalf("Register(assets) error = %v", err)
	}

	assertResource(t, store, "default.txt", "default")
	assertResource(t, store, "assets:named.txt", "named")
}

func TestStoreRequiresExplicitDefaultNamespace(t *testing.T) {
	store := New()
	first := fstest.MapFS{"value.txt": {Data: []byte("first")}}
	second := fstest.MapFS{"value.txt": {Data: []byte("second")}}
	if err := store.Register("first", first); err != nil {
		t.Fatalf("Register(first) error = %v", err)
	}
	if err := store.Register("second", second); err != nil {
		t.Fatalf("Register(second) error = %v", err)
	}
	if _, err := store.ReadFile("value.txt"); err == nil || err.Error() != `resource namespace "" is not registered` {
		t.Fatalf("ReadFile(unqualified) error = %v", err)
	}
	assertResource(t, store, "first:value.txt", "first")
	assertResource(t, store, "second:value.txt", "second")
}

func TestStoreRejectsDuplicateNamespace(t *testing.T) {
	store := New()
	source := fstest.MapFS{"value.txt": {Data: []byte("value")}}
	if err := store.Register("assets", source); err != nil {
		t.Fatalf("Register(assets) error = %v", err)
	}
	if err := store.Register("assets", source); err == nil {
		t.Fatal("Register(assets) expected duplicate namespace error")
	}
}

func TestStoreOpensNamespacedResource(t *testing.T) {
	store := New()
	if err := store.Register("assets", fstest.MapFS{"value.txt": {Data: []byte("value")}}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	file, err := store.Open("assets:value.txt")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil || string(data) != "value" {
		t.Fatalf("ReadAll() = %q, %v", data, err)
	}
}

func assertResource(t *testing.T, store *Store, reference, want string) {
	t.Helper()
	actual, err := store.ReadFile(reference)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", reference, err)
	}
	if string(actual) != want {
		t.Fatalf("ReadFile(%q) = %q, want %q", reference, actual, want)
	}
}
