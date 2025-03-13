package buildin

import (
	"context"
	"github.com/viant/bindly/locator"
	"github.com/viant/structology"
	"reflect"
)

type (
	DirectLocatorProvider struct {
		state    *structology.State
		priority int
		kind     string
	}

	directLocator struct {
		state *structology.State
		kind  string
	}
)

func (l *directLocator) Value(ctx context.Context, name string) (interface{}, bool, error) {
	value, err := l.state.Value(name)
	if err != nil {
		// The field doesn't exist
		return nil, false, nil
	}
	return value, true, nil
}

func (l *directLocator) Kind() string {
	return l.kind
}

func (p *DirectLocatorProvider) Locate(state *structology.State) locator.Locator {
	return &directLocator{state: p.state, kind: p.kind}
}

func (p *DirectLocatorProvider) Kind() string {
	return p.kind
}

func (p *DirectLocatorProvider) Priority() int {
	return p.priority
}

func Direct(kind string, value interface{}, priority int) locator.Provider {
	stateType := structology.NewStateType(reflect.TypeOf(value))
	state := stateType.WithValue(value)
	return &DirectLocatorProvider{
		state:    state,
		priority: priority,
		kind:     kind,
	}
}
