package resource

import (
	"testing"
	"testing/fstest"
)

func TestScopedDefaultSharesNamedAuthority(t *testing.T) {
	root := New()
	first, err := root.WithDefault(fstest.MapFS{"local.sql": {Data: []byte("first")}})
	if err != nil {
		t.Fatal(err)
	}
	second, err := first.WithDefault(fstest.MapFS{"local.sql": {Data: []byte("second")}})
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Register("shared", fstest.MapFS{"query.sql": {Data: []byte("shared")}}); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name  string
		store *Store
		local string
	}{{"root", root, ""}, {"first", first, "first"}, {"second", second, "second"}} {
		t.Run(test.name, func(t *testing.T) {
			value, err := test.store.ReadFile("shared:query.sql")
			if err != nil || string(value) != "shared" {
				t.Fatalf("named=%s %v", value, err)
			}
			value, err = test.store.ReadFile("local.sql")
			if test.local == "" {
				if err == nil {
					t.Fatal("scoped default leaked into root")
				}
			} else if err != nil || string(value) != test.local {
				t.Fatalf("local=%s %v", value, err)
			}
			if err := test.store.Register("shared", fstest.MapFS{}); err == nil {
				t.Fatal("duplicate namespace accepted")
			}
		})
	}
	if err := first.Register("", fstest.MapFS{}); err == nil {
		t.Fatal("scoped default can be changed")
	}
	if _, err := root.WithDefault(nil); err == nil {
		t.Fatal("nil scoped FS accepted")
	}
	if err := root.Register("", fstest.MapFS{"root.sql": {Data: []byte("root")}}); err != nil {
		t.Fatal(err)
	}
	if _, err := first.ReadFile("root.sql"); err == nil {
		t.Fatal("root default bypassed scoped default")
	}
}
