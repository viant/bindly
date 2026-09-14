package bindly

import (
	"context"
	"fmt"
	xshape "github.com/viant/x/shape"
	"reflect"
)

func (r *Replay) capturePrepared(ctx context.Context, input any, assigned map[string]bool) error {
	prepared := map[string]replayValue{}
	for _, path := range r.plan.order {
		binding := r.plan.fields[path]
		present, err := r.plan.plan.Presence(input, path)
		marker, markerErr := r.plan.plan.presenceMarker(input, path)
		if markerErr != nil {
			return markerErr
		}
		if marker == "" {
			present = assigned[path]
		}
		if err != nil {
			return err
		}
		if !present {
			if binding.Required != nil && *binding.Required {
				return &BindingError{Path: path, Code: binding.ErrorCode, Message: binding.ErrorMessage, Cause: fmt.Errorf("required replay input is absent")}
			}
			prepared[path] = replayValue{}
			delete(r.raw, binding.Name)
			continue
		}
		value, ok := r.plan.plan.fields[path].Value(reflect.ValueOf(input))
		if !ok {
			return fmt.Errorf("prepared replay input %s is inaccessible", path)
		}
		if binding.Required != nil && *binding.Required && nilValue(value.Interface()) {
			return &BindingError{Path: path, Code: binding.ErrorCode, Message: binding.ErrorMessage, Cause: fmt.Errorf("required replay input is null")}
		}
		detached, err := (xshape.Runtime{}).CloneValue(value.Interface())
		if err != nil {
			return err
		}
		prepared[path] = replayValue{value: detached, found: true}
		if binding.Transformer == nil {
			original, err := r.source(ctx, binding, r.plan.plan.fields[path].Type)
			if err != nil {
				return err
			}
			if !original.found || !reflect.DeepEqual(original.value, detached) {
				encoded, err := r.encodeField(binding, detached)
				if err != nil {
					return err
				}
				r.raw[binding.Name] = encoded
			}
		}
	}
	r.prepared = prepared
	return nil
}

func (r *Replay) authorize(ctx context.Context, input any, gate func(context.Context, any) error) error {
	before := map[string]replayValue{}
	for path, binding := range r.plan.preflight {
		if binding.Transformer == nil && !r.plan.protected[path] {
			continue
		}
		value, ok := r.plan.plan.fields[path].Value(reflect.ValueOf(input))
		if !ok {
			return fmt.Errorf("codec input %s is inaccessible", path)
		}
		copy, err := (xshape.Runtime{}).CloneValue(value.Interface())
		if err != nil {
			return err
		}
		present, err := r.plan.plan.Presence(input, path)
		if err != nil {
			return err
		}
		before[path] = replayValue{value: copy, found: present}
	}
	if err := gate(ctx, input); err != nil {
		return err
	}
	for path := range r.plan.preflight {
		expected, ok := before[path]
		if !ok {
			continue
		}
		value, ok := r.plan.plan.fields[path].Value(reflect.ValueOf(input))
		if !ok {
			return fmt.Errorf("codec input %s is inaccessible", path)
		}
		present, err := r.plan.plan.Presence(input, path)
		if err != nil {
			return err
		}
		if present != expected.found || !reflect.DeepEqual(value.Interface(), expected.value) {
			return fmt.Errorf("authorization cannot replace verified codec input/output %s; supply raw source values", path)
		}
	}
	return nil
}
