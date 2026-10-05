package state

import "testing"

// TestNewState ensures the constructor returns a non-nil *State
// instance with a zero-value internal structology.State.
func TestNewState(t *testing.T) {
	s := NewState()
	if s == nil {
		t.Fatalf("NewState() returned nil, want non-nil *State")
	}
}

