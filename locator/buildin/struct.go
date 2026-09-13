package buildin

import (
	"context"
	"github.com/viant/bindly/locator"
	"github.com/viant/structology"
	"reflect"
)

type (
	StructLocatorProvider struct {
		priority int
		selector string
		kind     string
	}
	structLocator struct {
		rootSelector string
		state        *structology.State
		kind         string
	}
)

func (l *structLocator) Value(ctx context.Context, targetType reflect.Type, name string) (interface{}, bool, error) {
	if l.state == nil {
		return nil, false, nil
	}
	aPath := l.rootSelector + "." + name
	if name == "" {
		aPath = l.rootSelector
	} else if l.rootSelector == "" {
		aPath = name
	}
	if aPath == "" {
		value := l.state.StatePtr()
		if value == nil {
			value = l.state.State()
		}
		return value, true, nil
	}
	if l.state.Type().Lookup(aPath) == nil {
		return nil, false, nil
	}
	selector, err := l.state.Selector(aPath)
	if err != nil {
		return nil, false, nil
	}
	ptr := l.state.Pointer()
	value := selector.Value(ptr)
	hasValue := selector.Has(ptr)
	return value, hasValue, nil
}

func (p *structLocator) Kind() string {
	return p.kind
}

func (p *StructLocatorProvider) Locate(state *structology.State) locator.Locator {
	return &structLocator{state: state, rootSelector: p.selector, kind: p.kind}
}

func (p *StructLocatorProvider) Kind() string {
	return p.kind
}

func (p *StructLocatorProvider) Priority() int {
	return p.priority
}

func Struct(kind string, selector string, priority int) locator.Provider {
	return &StructLocatorProvider{
		kind:     kind,
		selector: selector,
		priority: priority,
	}
}
