package buildin

import (
	"context"
	"fmt"
	"github.com/viant/bindly/locator"
	"github.com/viant/structology"
)

type (
	MapLocatorProvider struct {
		priority int
		selector string
		kind     string
	}
	mapLocator struct {
		rootSelector string
		state        *structology.State
		kind         string
	}
)

func (l *mapLocator) Value(ctx context.Context, name string) (interface{}, bool, error) {
	value, err := l.state.Value(l.rootSelector)
	if err != nil {
		return nil, false, err
	}
	iFaces, ok := value.(map[string]interface{})
	if !ok {
		return nil, false, fmt.Errorf("expected map[string]interface{} but had %T", value)
	}
	result, ok := iFaces[name]
	return result, ok, nil
}

func (p *mapLocator) Kind() string {
	return p.kind
}

func (p *MapLocatorProvider) Locate(state *structology.State) locator.Locator {
	return &mapLocator{state: state, rootSelector: p.selector, kind: p.kind}
}

func (p *MapLocatorProvider) Kind() string {
	return p.kind
}

func (p *MapLocatorProvider) Priority() int {
	return p.priority
}

func Map(kind string, selector string, priority int) locator.Provider {
	return &MapLocatorProvider{
		kind:     kind,
		selector: selector,
		priority: priority,
	}
}
