package bindly

import "strings"

// ResolutionGroupError retains real failures in binding completion order.
// Returned slices and BindingError records are copies; causes are retained.
type ResolutionGroupError struct{ failures []*BindingError }

func (e *ResolutionGroupError) Failures() []*BindingError {
	if e == nil {
		return nil
	}
	result := make([]*BindingError, len(e.failures))
	for i, f := range e.failures {
		result[i] = copyBindingError(f)
	}
	return result
}
func (e *ResolutionGroupError) Unwrap() []error {
	if e == nil {
		return nil
	}
	result := make([]error, len(e.failures))
	for i, f := range e.failures {
		result[i] = copyBindingError(f)
	}
	return result
}
func (e *ResolutionGroupError) Error() string {
	if e == nil {
		return ""
	}
	parts := make([]string, len(e.failures))
	for i, f := range e.failures {
		parts[i] = f.Error()
	}
	return strings.Join(parts, "\n")
}
func copyBindingError(e *BindingError) *BindingError {
	if e == nil {
		return nil
	}
	result := *e
	return &result
}
