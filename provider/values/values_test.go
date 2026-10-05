package values

import (
	"context"
	"reflect"
	"testing"
	"time"
)

func TestTypedValues(t *testing.T) {
	for _, tt := range []struct {
		name   string
		value  any
		target reflect.Type
		want   any
		reject bool
	}{
		{"int", "17", reflect.TypeOf(int8(0)), int8(17), false},
		{"bool", "true", reflect.TypeOf(false), true, false},
		{"time", "2026-09-12T00:00:00Z", reflect.TypeOf(time.Time{}), time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC), false},
		{"slice", []string{"1", "2"}, reflect.TypeOf([]int{}), []int{1, 2}, false},
		{"overflow", "128", reflect.TypeOf(int8(0)), nil, true},
		{"nil", nil, reflect.TypeOf(0), nil, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			values := map[string]any{"value": tt.value}
			provider := New("authored", values)
			values["value"] = "changed"
			value, found, err := provider.Locate(nil).Value(context.Background(), tt.target, "value")
			if !found {
				t.Fatal("lost presence")
			}
			if tt.reject {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil || !reflect.DeepEqual(value, tt.want) {
				t.Fatalf("value %#v err %v", value, err)
			}
			if _, found, err := provider.Locate(nil).Value(context.Background(), tt.target, "missing"); found || err != nil {
				t.Fatalf("missing found %v err %v", found, err)
			}
		})
	}
}
