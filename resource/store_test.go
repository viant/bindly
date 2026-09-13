package resource

import (
	"io/fs"
	"testing"
	"testing/fstest"
)

func TestStore(t *testing.T) {
	s := New()
	source := fstest.MapFS{"query.sql": {Data: []byte("SELECT 1")}}
	if err := s.Register("queries", source); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		reference string
		ok        bool
	}{{"queries:query.sql", true}, {"query.sql", false}, {"queries:missing", false}, {"queries:../query.sql", false}, {"other:query.sql", false}} {
		t.Run(tt.reference, func(t *testing.T) {
			data, err := fs.ReadFile(s, tt.reference)
			if tt.ok {
				if err != nil || string(data) != "SELECT 1" {
					t.Fatalf("data %q error %v", data, err)
				}
			} else if err == nil {
				t.Fatal("expected error")
			}
		})
	}
	if err := s.Register("queries", source); err == nil {
		t.Fatal("duplicate accepted")
	}
	if err := s.Register("", source); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReadFile("query.sql"); err != nil {
		t.Fatal(err)
	}
}
