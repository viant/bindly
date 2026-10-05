package bindly

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/viant/bindly/provider/body"
	"github.com/viant/bindly/xform/conv"
	"reflect"
)

// ReplayPlan is an immutable selection of external fields from one canonical
// binding plan. It retains no invocation values or dependency-read evidence.
type ReplayPlan struct {
	plan      *Plan
	fields    map[string]BindingSpec
	order     []string
	preflight map[string]BindingSpec
	protected map[string]bool
}

// Replay holds one detached set of source values. Prepared values can only be
// produced by successful native binding. It is invocation-owned, not shareable.
type Replay struct {
	plan     *ReplayPlan
	raw      map[string]json.RawMessage
	prepared map[string]replayValue
}
type replayValue struct {
	value any
	found bool
}
type ReplayBinding struct {
	Replay *Replay
	Only   bool
	Gate   func(context.Context, any) error
}

func WithReplay(binding ReplayBinding) BindOption {
	return func(o *bindOptions) { o.replay = &binding }
}

func (p *ReplayPlan) Capture(input any) (*Replay, error) {
	value := reflect.ValueOf(input)
	if !value.IsValid() || value.Kind() != reflect.Pointer || value.IsNil() || value.Elem().Type() != p.plan.target {
		return nil, fmt.Errorf("replay input must be *%s", p.plan.target)
	}
	result := &Replay{plan: p, raw: map[string]json.RawMessage{}}
	for _, path := range p.order {
		binding := p.fields[path]
		present, err := p.plan.Presence(input, path)
		if err != nil {
			return nil, err
		}
		if !present {
			if binding.Required != nil && *binding.Required {
				return nil, &BindingError{Path: path, Code: binding.ErrorCode, Message: binding.ErrorMessage, Cause: fmt.Errorf("required replay input is absent")}
			}
			continue
		}
		if binding.Transformer != nil {
			return nil, fmt.Errorf("replay parameter %s requires raw source JSON; decoded codec values cannot be persisted as source", binding.Name)
		}
		field, ok := p.plan.fields[path].Value(value)
		if !ok {
			return nil, fmt.Errorf("replay field %s is inaccessible", path)
		}
		encoded, err := result.encodeField(binding, field.Interface())
		if err != nil {
			return nil, err
		}
		result.raw[binding.Name] = encoded
	}
	return result, nil
}
func (p *ReplayPlan) DecodeJSON(data []byte) (*Replay, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	if raw == nil {
		return nil, fmt.Errorf("replay state must be a JSON object")
	}
	result := &Replay{plan: p, raw: map[string]json.RawMessage{}}
	for _, binding := range p.fields {
		if value, ok := raw[binding.Name]; ok {
			result.raw[binding.Name] = append(json.RawMessage(nil), value...)
		}
	}
	return result, nil
}
func (r *Replay) MarshalJSON() ([]byte, error) { return json.Marshal(r.raw) }
func (r *Replay) Plan() *Plan                  { return r.plan.plan }
func (r *Replay) source(ctx context.Context, binding BindingSpec, target reflect.Type) (resolution, error) {
	if binding.Transformer != nil && binding.SourceType == nil {
		return resolution{}, fmt.Errorf("replay codec %s requires a source type", binding.Name)
	}
	raw, found := r.raw[binding.Name]
	if !found {
		return resolution{}, nil
	}
	decoder, err := body.New(raw, "application/json", nil)
	if err != nil {
		return resolution{}, err
	}
	value, found, err := decoder.Value(ctx, target, "")
	return resolution{value: value, found: found}, err
}
func (r *Replay) encodeField(binding BindingSpec, value any) ([]byte, error) {
	sourceType := binding.SourceType
	if sourceType == nil {
		sourceType = r.plan.plan.fields[binding.Path].Type
	}
	source, err := (conv.ValueConverter{}).Convert(value, sourceType)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(source)
	if err != nil {
		return nil, err
	}
	decoder, err := body.New(data, "application/json", nil)
	if err != nil {
		return nil, err
	}
	decoded, _, err := decoder.Value(context.Background(), r.plan.plan.fields[binding.Path].Type, "")
	if err != nil {
		return nil, err
	}
	if decoded == nil {
		decoded = reflect.Zero(r.plan.plan.fields[binding.Path].Type).Interface()
	}
	if !reflect.DeepEqual(value, decoded) {
		return nil, fmt.Errorf("replay parameter %s cannot preserve typed value/presence through JSON; provide raw source JSON", binding.Name)
	}
	return data, nil
}
