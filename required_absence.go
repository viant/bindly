package bindly

import (
	"fmt"
	"github.com/viant/bindly/input"
)

// requiredAbsenceError retains the original private diagnostic for an external
// input's own absence decision. It is never inferred from a dependency failure.
type requiredAbsenceError struct {
	cause error
	owner *BindingError
}

func (e *requiredAbsenceError) Error() string { return e.cause.Error() }
func (e *requiredAbsenceError) Unwrap() error { return e.cause }

func (b BindingSpec) requiredAbsence() (*requiredAbsenceError, error) {
	diagnostic := fmt.Errorf("missing required %s value %q", b.Location.Kind, b.Location.In)
	err := b.inputError(diagnostic)
	invalid, external := err.(*input.Error)
	if !external {
		return nil, err
	}
	absent := &requiredAbsenceError{cause: diagnostic}
	invalid.Cause = absent
	return absent, invalid
}

func (e *BindingError) requiredAbsenceMessage() (string, bool) {
	// Direct cause and owner identity refer only to this binding's absence event.
	// Do not traverse dependency errors or alter BindingError's public shape.
	invalid, ok := e.Unwrap().(*input.Error)
	if !ok {
		return "", false
	}
	absent, ok := invalid.Cause.(*requiredAbsenceError)
	if !ok || absent.owner != e {
		return "", false
	}
	name := e.Name
	if name == "" {
		name = e.Path
	}
	return fmt.Sprintf("parameter %s is required", name), true
}
