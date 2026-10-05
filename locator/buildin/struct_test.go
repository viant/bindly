package buildin

import (
	"context"
	"reflect"
	"testing"

	"github.com/viant/structology"
)

// Sample is a simple struct used to validate Struct locator behavior.
type Sample struct {
	Name    string
	Age     int
	Address struct {
		City string
	}
}

// TestStructLocator_Value verifies that Struct locator uses a structology.State
// built in the same way as production code (see direct.go) and that field
// resolution behaves as expected.
func TestStructLocator_Value(t *testing.T) {
	value := &Sample{Name: "Alice", Age: 30}
	value.Address.City = "Warsaw"

	stateType := structology.NewStateType(reflect.TypeOf(Sample{}))
	state := stateType.WithValue(value)

	provider := Struct("sample", "", 1).(*StructLocatorProvider)
	locator := provider.Locate(state)

	tests := []struct {
		name      string
		fieldName string
		wantValue interface{}
		wantOK    bool
		wantErr   bool
	}{
		{
			name:      "existing field",
			fieldName: "Name",
			wantValue: "Alice",
			wantOK:    true,
			wantErr:   false,
		},
		{
			name:      "missing field",
			fieldName: "Unknown",
			wantValue: nil,
			wantOK:    false,
			wantErr:   false,
		},
		{
			name:      "nested field",
			fieldName: "Address.City",
			wantValue: "Warsaw",
			wantOK:    true,
			wantErr:   false,
		},
		{
			name:      "empty name selects root struct",
			fieldName: "",
			wantValue: value,
			wantOK:    true,
			wantErr:   false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok, err := locator.Value(context.Background(), nil, tc.fieldName)
			if (err != nil) != tc.wantErr {
				t.Fatalf("Value() error = %v, wantErr %v", err, tc.wantErr)
			}
			if err != nil {
				return
			}
			if ok != tc.wantOK {
				t.Fatalf("Value() ok = %v, want %v", ok, tc.wantOK)
			}
			if ok && !reflect.DeepEqual(got, tc.wantValue) {
				t.Fatalf("Value() got = %#v, want %#v", got, tc.wantValue)
			}
		})
	}
}

func TestStructLocatorHonorsSourcePresenceMarker(t *testing.T) {
	type sampleHas struct {
		Name bool
	}
	type markedSample struct {
		Name string
		Has  *sampleHas `setMarker:"true"`
	}

	for _, testCase := range []struct {
		name  string
		value *markedSample
		found bool
	}{
		{name: "present", value: &markedSample{Name: "Alice", Has: &sampleHas{Name: true}}, found: true},
		{name: "absent", value: &markedSample{Name: "Alice", Has: &sampleHas{}}, found: false},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			state := structology.NewStateType(reflect.TypeOf(testCase.value)).WithValue(testCase.value)
			actual, found, err := Struct("sample", "", 1).Locate(state).Value(context.Background(), nil, "Name")
			if err != nil || found != testCase.found {
				t.Fatalf("Value(Name) = (%v, %v, %v), want found=%v", actual, found, err, testCase.found)
			}
			if found && actual != "Alice" {
				t.Fatalf("Value(Name) = %v, want Alice", actual)
			}
		})
	}
}
