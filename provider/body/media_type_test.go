package body

import (
	"context"
	"reflect"
	"testing"
	"time"
)

func TestBodyMediaTypeContract(t *testing.T) {
	bytes := []byte(`[1]`)
	for _, tt := range []struct {
		name, mediaType, raw string
		target               reflect.Type
		want                 any
		reject               bool
	}{
		{"json object", "application/json", `{"id":1}`, reflect.TypeOf(map[string]int{}), map[string]int{"id": 1}, false},
		{"vendor json", "application/vnd.test+json", `{"id":1}`, reflect.TypeOf(map[string]int{}), map[string]int{"id": 1}, false},
		{"text is not json", "text/plain", `{"id":1}`, reflect.TypeOf(map[string]int{}), nil, true},
		{"binary is not json", "application/octet-stream", `[1]`, reflect.TypeOf([]int{}), nil, true},
		{"raw text", "text/plain", `{"id":1}`, reflect.TypeOf(""), `{"id":1}`, false},
		{"raw bytes", "application/octet-stream", `[1]`, reflect.TypeOf([]byte{}), []byte(`[1]`), false},
		{"scalar conversion", "text/plain", "42", reflect.TypeOf(0), 42, false},
		{"pointer raw bytes", "application/octet-stream", `[1]`, reflect.TypeOf((*[]byte)(nil)), &bytes, false},
		{"time scalar", "text/plain", "2026-09-12T00:00:00Z", reflect.TypeOf(time.Time{}), time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC), false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			source, err := New([]byte(tt.raw), tt.mediaType, nil)
			if err != nil {
				t.Fatal(err)
			}
			actual, found, err := source.Value(context.Background(), tt.target, "")
			if tt.reject {
				if err == nil {
					t.Fatal("incorrect media type accepted")
				}
				return
			}
			if err != nil || !found || !reflect.DeepEqual(actual, tt.want) {
				t.Fatalf("actual=%v found=%t error=%v", actual, found, err)
			}
		})
	}
}
