package buildin

import (
	"context"
	"fmt"
	"github.com/viant/bindly/locator"
	"github.com/viant/structology"
	"reflect"
	"strings"
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

func (l *mapLocator) Value(ctx context.Context, targetType reflect.Type, name string) (interface{}, bool, error) {
	if l.state == nil {
		return nil, false, nil
	}
	// Map values must use Go's map access, not an unsafe map header fabricated
	// by older structural selectors (whose layout changed in Go 1.24).
	value := reflect.ValueOf(l.state.State())
	for _, field := range strings.Split(l.rootSelector, ".") {
		for value.IsValid() && (value.Kind() == reflect.Pointer || value.Kind() == reflect.Interface) {
			if value.IsNil() {
				return nil, false, nil
			}
			value = value.Elem()
		}
		if field == "" {
			continue
		}
		if !value.IsValid() || value.Kind() != reflect.Struct {
			return nil, false, fmt.Errorf("map source %s is not a struct field", l.rootSelector)
		}
		value = value.FieldByName(field)
	}
	for value.IsValid() && value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return nil, false, nil
		}
		value = value.Elem()
	}
	if !value.IsValid() || value.Kind() != reflect.Map || value.Type().Key().Kind() != reflect.String {
		return nil, false, fmt.Errorf("map source %s requires string keys", l.rootSelector)
	}
	result := value.MapIndex(reflect.ValueOf(name).Convert(value.Type().Key()))
	if !result.IsValid() {
		return nil, false, nil
	}
	return result.Interface(), true, nil
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
