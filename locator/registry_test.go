package locator

import (
	"testing"

	"github.com/viant/structology"
)

// TestRegistry_Register verifies basic registration semantics including
// success path and duplicate-kind error behavior.
func TestRegistry_Register(t *testing.T) {
	tests := []struct {
		name        string
		prepare     func(r *Registry)
		provider    Provider
		wantErr     bool
		wantLen     int
		lookupKind  string
		wantPresent bool
	}{
		{
			name:    "register new kind succeeds",
			prepare: func(r *Registry) {},
			provider: &mockProvider{
				kind:     "kindA",
				priority: 1,
			},
			wantErr:     false,
			wantLen:     1,
			lookupKind:  "kindA",
			wantPresent: true,
		},
		{
			name: "duplicate kind returns error and keeps original",
			prepare: func(r *Registry) {
				_ = r.Register(&mockProvider{kind: "dup", priority: 1})
			},
			provider: &mockProvider{
				kind:     "dup",
				priority: 2,
			},
			wantErr:     true,
			wantLen:     1,
			lookupKind:  "dup",
			wantPresent: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			reg := NewRegistry()
			if tc.prepare != nil {
				tc.prepare(reg)
			}

			gotErr := reg.Register(tc.provider)
			if (gotErr != nil) != tc.wantErr {
				t.Fatalf("Register() error = %v, wantErr %v", gotErr, tc.wantErr)
			}

			// use internal map length via Keys() helper from embedded Map
			if gotLen := len(reg.Keys()); gotLen != tc.wantLen {
				t.Fatalf("registry size = %d, want %d", gotLen, tc.wantLen)
			}

			if tc.lookupKind != "" {
				_, ok := reg.Lookup(tc.lookupKind)
				if ok != tc.wantPresent {
					t.Fatalf("Lookup(%q) ok = %v, want %v", tc.lookupKind, ok, tc.wantPresent)
				}
			}
		})
	}
}

// TestRegistry_UnregisterAndLookup exercises Unregister and Lookup helpers.
func TestRegistry_UnregisterAndLookup(t *testing.T) {
	reg := NewRegistry()
	prov := &mockProvider{kind: "toRemove", priority: 1}
	if err := reg.Register(prov); err != nil {
		t.Fatalf("unexpected error registering provider: %v", err)
	}

	if _, ok := reg.Lookup("toRemove"); !ok {
		t.Fatalf("Lookup before Unregister returned ok = false, want true")
	}

	reg.Unregister("toRemove")
	if _, ok := reg.Lookup("toRemove"); ok {
		t.Fatalf("Lookup after Unregister returned ok = true, want false")
	}
}

// mockProvider is a lightweight Provider implementation used in tests.
type mockProvider struct {
	kind     string
	priority int
}

func (m *mockProvider) Locate(_ *structology.State) Locator { return nil }

func (m *mockProvider) Kind() string { return m.kind }

func (m *mockProvider) Priority() int { return m.priority }
