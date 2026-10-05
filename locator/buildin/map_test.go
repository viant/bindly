package buildin

import (
	"context"
	"testing"
)

// TestMapLocator_Value covers successful lookup, missing key, and
// type-assertion failure when the root selector does not point to a
// map[string]interface{} value.
func TestMapLocator_Value(t *testing.T) {
	type stateWrapper struct {
		Instances map[string]interface{}
		Other     string
	}

	type sample struct {
		Name string
	}

	stateVal := &stateWrapper{
		Instances: map[string]interface{}{
			"foo": 42,
			"bar": &sample{Name: "baz"},
		},
		Other: "not a map",
	}

	// Map provider expects a *structology.State; build it using Direct.
	stateProvider := Direct("wrapper", stateVal, 1).(*DirectLocatorProvider)

	tests := []struct {
		name       string
		selector   string
		lookupName string
		wantValue  interface{}
		wantOK     bool
		wantErr    bool
	}{
		{
			name:       "existing key returns value",
			selector:   "Instances",
			lookupName: "foo",
			wantValue:  42,
			wantOK:     true,
			wantErr:    false,
		},
		{
			name:       "missing key returns ok=false without error",
			selector:   "Instances",
			lookupName: "missing",
			wantValue:  nil,
			wantOK:     false,
			wantErr:    false,
		},
		{
			name:       "selector not a map produces error",
			selector:   "Other",
			lookupName: "foo",
			wantOK:     false,
			wantErr:    true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			provider := Map("instance", tc.selector, 1).(*MapLocatorProvider)
			locator := provider.Locate(stateProvider.state)

			got, ok, err := locator.Value(context.Background(), nil, tc.lookupName)
			if (err != nil) != tc.wantErr {
				t.Fatalf("Value() error = %v, wantErr %v", err, tc.wantErr)
			}
			if tc.wantErr {
				return
			}
			if ok != tc.wantOK {
				t.Fatalf("Value() ok = %v, want %v", ok, tc.wantOK)
			}
			if ok && got != tc.wantValue {
				t.Fatalf("Value() got = %v, want %v", got, tc.wantValue)
			}
		})
	}
}
