package bindly

import (
	"fmt"
	"strings"
)

// BindingError reports a value-resolution failure for one compiled binding.
// Code and Message come from authored binding metadata when supplied.
type BindingError struct {
	Path    string
	Name    string
	Kind    string
	In      string
	Code    int
	Message string
	Err     error
}

func (e *BindingError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return fmt.Sprintf("binding %s failed", e.bindingName())
}

func (e *BindingError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// StatusCode exposes an authored error status to protocol adapters without
// coupling Bindly to any transport package.
func (e *BindingError) StatusCode() int {
	if e == nil {
		return 0
	}
	return e.Code
}

func (e *BindingError) bindingName() string {
	if e.Name != "" {
		return e.Name
	}
	return e.Path
}

func newBindingError(binding *Binding, err error) error {
	if binding == nil {
		return err
	}
	message := strings.TrimSpace(binding.ErrorMessage)
	if message != "" {
		cause := ""
		if err != nil {
			cause = err.Error()
		}
		message = strings.ReplaceAll(message, "${error}", cause)
	}
	return &BindingError{
		Path:    binding.destinationPath(),
		Name:    binding.Name,
		Kind:    binding.Kind(),
		In:      binding.In(),
		Code:    binding.ErrorCode,
		Message: message,
		Err:     err,
	}
}
