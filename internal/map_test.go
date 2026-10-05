package internal

import "testing"

// TestMapBasicOperations verifies basic CRUD-like behavior of Map.
func TestMapBasicOperations(t *testing.T) {
	type entry struct {
		key   string
		value int
	}

	tests := []struct {
		name       string
		prepare    []entry
		lookupKey  string
		wantValue  int
		wantOK     bool
		deleteKey  string
		wantExists bool
	}{
		{
			name:       "put and get existing key",
			prepare:    []entry{{"a", 1}},
			lookupKey:  "a",
			wantValue:  1,
			wantOK:     true,
			deleteKey:  "a",
			wantExists: false,
		},
		{
			name:      "get missing key",
			prepare:   nil,
			lookupKey: "missing",
			wantOK:    false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := NewMap[string, int]()
			for _, e := range tc.prepare {
				m.Put(e.key, e.value)
			}

			got, ok := m.Get(tc.lookupKey)
			if ok != tc.wantOK {
				t.Fatalf("Get(%q) ok = %v, want %v", tc.lookupKey, ok, tc.wantOK)
			}
			if ok && got != tc.wantValue {
				t.Fatalf("Get(%q) value = %v, want %v", tc.lookupKey, got, tc.wantValue)
			}

			if tc.deleteKey != "" {
				m.Delete(tc.deleteKey)
				if exists := m.Exists(tc.deleteKey); exists != tc.wantExists {
					t.Fatalf("Exists(%q) = %v, want %v", tc.deleteKey, exists, tc.wantExists)
				}
			}
		})
	}
}

// TestMapLenKeysCloneRange exercises auxiliary Map helpers.
func TestMapLenKeysCloneRange(t *testing.T) {
	m := NewMap[int, string]()
	for i := 0; i < 5; i++ {
		m.Put(i, string(rune('a'+i)))
	}

	if got, want := m.Len(), 5; got != want {
		t.Fatalf("Len() = %d, want %d", got, want)
	}

	keys := m.Keys()
	if len(keys) != 5 {
		t.Fatalf("Keys() len = %d, want 5", len(keys))
	}

	cloned := m.Clone()
	if cloned.Len() != m.Len() {
		t.Fatalf("Clone().Len() = %d, want %d", cloned.Len(), m.Len())
	}

	// Ensure Range visits entries until callback returns false.
	count := 0
	m.Range(func(key int, value string) bool {
		count++
		return count < 3
	})
	if count != 3 {
		t.Fatalf("Range visited %d entries, want 3", count)
	}
}

func TestMapLoadOrStoreKeepsFirstValue(t *testing.T) {
	m := NewMap[string, int]()
	actual, loaded := m.LoadOrStore("key", 1)
	if loaded || actual != 1 {
		t.Fatalf("first LoadOrStore() = (%d, %v), want (1, false)", actual, loaded)
	}
	actual, loaded = m.LoadOrStore("key", 2)
	if !loaded || actual != 1 {
		t.Fatalf("second LoadOrStore() = (%d, %v), want (1, true)", actual, loaded)
	}
}

func TestMapClear(t *testing.T) {
	m := NewMap[string, int]()
	m.Put("key", 1)
	m.Clear()
	if m.Len() != 0 {
		t.Fatalf("Len() = %d after Clear(), want 0", m.Len())
	}
}
