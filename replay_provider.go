package bindly

import (
	"context"
	"fmt"
	"github.com/viant/bindly/locator"
	"github.com/viant/bindly/xform/conv"
	"github.com/viant/structology"
	xshape "github.com/viant/x/shape"
	"reflect"
	"sort"
)

type replayProvider struct {
	replay *Replay
	kind   string
	fields map[string]BindingSpec
}

// Providers exposes each selected source kind independently to dependency
// components. Missing selected inputs are authoritative absence, not fallback.
func (r *Replay) Providers() ([]locator.Provider, error) {
	groups := map[string]*replayProvider{}
	for _, binding := range r.plan.fields {
		kind := binding.Location.Kind
		p := groups[kind]
		if p == nil {
			p = &replayProvider{replay: r, kind: kind, fields: map[string]BindingSpec{}}
			groups[kind] = p
		}
		if previous, ok := p.fields[binding.Location.In]; ok && previous.Path != binding.Path {
			return nil, fmt.Errorf("ambiguous replay location %s:%s", kind, binding.Location.In)
		}
		p.fields[binding.Location.In] = binding
	}
	kinds := make([]string, 0, len(groups))
	for kind := range groups {
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)
	result := make([]locator.Provider, 0, len(kinds))
	for _, kind := range kinds {
		result = append(result, groups[kind])
	}
	return result, nil
}
func (p *replayProvider) Kind() string                              { return p.kind }
func (p *replayProvider) Priority() int                             { return locator.PrioritySource }
func (p *replayProvider) DefaultCacheable() bool                    { return false }
func (p *replayProvider) Locate(*structology.State) locator.Locator { return p }
func (p *replayProvider) Owns(name string) bool                     { _, ok := p.fields[name]; return ok }
func (p *replayProvider) Value(ctx context.Context, target reflect.Type, name string) (any, bool, error) {
	binding, ok := p.fields[name]
	if !ok {
		return nil, false, nil
	}
	if value, prepared := p.replay.prepared[binding.Path]; prepared && binding.Transformer == nil {
		if !value.found {
			return nil, false, nil
		}
		detached, err := (xshape.Runtime{}).CloneValue(value.value)
		if err != nil {
			return nil, false, err
		}
		converted, err := (conv.ValueConverter{}).Convert(detached, target)
		return converted, true, err
	}
	result, err := p.replay.source(ctx, binding, target)
	return result.value, result.found, err
}
