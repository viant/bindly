package bindly

import (
	"context"
	"fmt"
	"reflect"

	"github.com/viant/bindly/locator"
	"github.com/viant/bindly/state"
	"github.com/viant/bindly/xform/conv"
)

// BindingEvent describes one successfully assigned target field. Target is the
// actual Bind/BindTarget destination, not necessarily the initial input. Path is
// that target's full planned field selector. Value is the assigned value;
// Metadata is nil when its provenance is unknown or an opaque conversion ran.
// Later bindings served by the explicit persistent value cache also have nil
// metadata: opaque evidence is never persisted or reconstructed from a value.
// No values or metadata are logged automatically.
type BindingEvent struct {
	Target   any
	Path     string
	Location state.Location
	Value    any
	Metadata any
}

// BindingObserver runs after field and presence-marker assignment. An error
// aborts binding through BindingError; already applied assignments are not
// rolled back. Observers own any longer-lived recording or publication policy.
type BindingObserver func(context.Context, BindingEvent) error

func WithBindingObserver(observer BindingObserver) BindOption {
	return func(options *bindOptions) { options.observer = observer }
}

func (s *invocation) MetadataRequested() bool { return s != nil && s.observer != nil }

func (r *resolution) unwrap(requested bool) error {
	switch envelope := r.value.(type) {
	case locator.ValueWithMetadata:
		r.value, r.metadata = envelope.Value, envelope.Metadata
	case *locator.ValueWithMetadata:
		if envelope == nil {
			return fmt.Errorf("provider returned a nil metadata envelope")
		}
		r.value, r.metadata = envelope.Value, envelope.Metadata
	}
	switch r.value.(type) {
	case locator.ValueWithMetadata, *locator.ValueWithMetadata:
		return fmt.Errorf("provider returned nested metadata envelopes")
	}
	if !requested || !r.found {
		r.metadata = nil
	}
	return nil
}

func (r *resolution) convert(target reflect.Type) error {
	// Native ValueConverter preserves the exact supplied value only for nil
	// target/assignable input. All other adaptations lose opaque evidence.
	if target != nil && (r.value == nil || !reflect.TypeOf(r.value).AssignableTo(target)) {
		r.metadata = nil
	}
	var err error
	r.value, err = (conv.ValueConverter{}).Convert(r.value, target)
	if err != nil {
		r.metadata = nil
	}
	return err
}

func (s *invocation) observed(ctx context.Context, target any, binding BindingSpec, result resolution) error {
	if s.observer == nil {
		return nil
	}
	if err := s.observer(ctx, BindingEvent{Target: target, Path: binding.Path, Location: binding.Location, Value: result.value, Metadata: result.metadata}); err != nil {
		return &BindingError{Path: binding.Path, Code: binding.ErrorCode, Message: binding.ErrorMessage, Cause: err}
	}
	return nil
}
