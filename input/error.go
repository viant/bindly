// Package input owns typed failures of external value decoding and conversion.
package input

import (
	"errors"
	"net/http"
)

// Error retains private decoding details while exposing a stable client status.
type Error struct {
	Code  int
	Cause error
}

func (e *Error) Error() string { return http.StatusText(e.StatusCode()) }
func (e *Error) StatusCode() int {
	if e.Code != 0 {
		return e.Code
	}
	var coder interface{ StatusCode() int }
	if errors.As(e.Cause, &coder) && coder.StatusCode() != 0 {
		return coder.StatusCode()
	}
	return http.StatusBadRequest
}
func (e *Error) Unwrap() error { return e.Cause }
