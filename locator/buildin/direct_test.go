package buildin

import (
	"context"
	"testing"
)

// TestDirectLocator_Value verifies direct field lookup behavior,
// including the no-error path when the field does not exist.
func TestDirectLocator_Value(t *testing.T) {
	type sample struct {
		Foo int
	}

	val := &sample{Foo: 10}
	// Direct already builds the underlying structology.State internally,
	// so we only need the provider and the locator it returns.
	provider := Direct("direct", val, 1).(*DirectLocatorProvider)
	locator := provider.Locate(provider.state)

	tests := []struct {
		name       string
		lookupName string
		wantValue  interface{}
		wantOK     bool
	}{
		{
			name:       "existing field returns value",
			lookupName: "Foo",
			wantValue:  10,
			wantOK:     true,
		},
		{
			name:       "missing field returns ok=false and nil value",
			lookupName: "Bar",
			wantValue:  nil,
			wantOK:     false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok, err := locator.Value(context.Background(), nil, tc.lookupName)
			if err != nil {
				t.Fatalf("Value() unexpected error: %v", err)
			}
			if ok != tc.wantOK {
				t.Fatalf("Value() ok = %v, want %v", ok, tc.wantOK)
			}
			if ok && got != tc.wantValue {
				t.Fatalf("Value() got = %v, want %v", got, tc.wantValue)
			}
			if !ok && got != nil {
				t.Fatalf("Value() got = %v, want nil when ok=false", got)
			}
		})
	}
}
