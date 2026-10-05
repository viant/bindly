// Package values supplies an invocation-scoped source of authored values.
package values

import (
	"context"
	"reflect"

	"github.com/viant/bindly/locator"
	"github.com/viant/bindly/xform/conv"
	"github.com/viant/structology"
)

type provider struct {
	kind   string
	values map[string]any
}

func New(kind string, values map[string]any) locator.Provider {
	copy := make(map[string]any, len(values))
	for name, value := range values {
		copy[name] = value
	}
	return &provider{kind: kind, values: copy}
}
func (p *provider) Kind() string                              { return p.kind }
func (p *provider) Priority() int                             { return 0 }
func (p *provider) DefaultCacheable() bool                    { return true }
func (p *provider) Locate(*structology.State) locator.Locator { return p }
func (p *provider) Value(_ context.Context, target reflect.Type, name string) (any, bool, error) {
	value, ok := p.values[name]
	if !ok {
		return nil, false, nil
	}
	converted, err := (conv.ValueConverter{}).Convert(value, target)
	return converted, true, err
}
