package input

import (
	"errors"
	"testing"
)

type statusError struct{}

func (statusError) Error() string   { return "PRIVATE" }
func (statusError) StatusCode() int { return 422 }
func TestStatusPreservesTypedPolicy(t *testing.T) {
	for _, tc := range []struct {
		code  int
		cause error
		want  int
	}{{0, errors.New("PRIVATE"), 400}, {0, statusError{}, 422}, {401, statusError{}, 401}} {
		err := &Error{Code: tc.code, Cause: tc.cause}
		if err.StatusCode() != tc.want || err.Error() == "PRIVATE" || !errors.Is(err, tc.cause) {
			t.Fatalf("error=%v status=%d", err, err.StatusCode())
		}
	}
}
