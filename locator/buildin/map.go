package buildin

import (
	"context"
	"fmt"
	"reflect"
	"strings"

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
		source       interface{}
		kind         string
	}
)

func (l *mapLocator) Value(ctx context.Context, _ reflect.Type, name string) (interface{}, bool, error) {
	value, err := mapSourceValue(l.source, l.rootSelector)
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
	if state == nil {
		return nil
	}
	source := state.StatePtr()
	if source == nil {
		source = state.State()
	}
	return &mapLocator{source: source, rootSelector: p.selector, kind: p.kind}
}

func mapSourceValue(source interface{}, path string) (interface{}, error) {
	value := reflect.ValueOf(source)
	for _, segment := range strings.Split(strings.TrimSpace(path), ".") {
		for value.IsValid() && (value.Kind() == reflect.Ptr || value.Kind() == reflect.Interface) {
			if value.IsNil() {
				return nil, fmt.Errorf("map selector %q traverses nil", path)
			}
			value = value.Elem()
		}
		if segment == "" {
			continue
		}
		if !value.IsValid() || value.Kind() != reflect.Struct {
			return nil, fmt.Errorf("map selector %q traverses %s", path, value.Kind())
		}
		value = value.FieldByName(segment)
		if !value.IsValid() {
			return nil, fmt.Errorf("map selector %q was not found", path)
		}
	}
	for value.IsValid() && (value.Kind() == reflect.Ptr || value.Kind() == reflect.Interface) {
		if value.IsNil() {
			return nil, nil
		}
		value = value.Elem()
	}
	if !value.IsValid() || !value.CanInterface() {
		return nil, fmt.Errorf("map selector %q is not accessible", path)
	}
	return value.Interface(), nil
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
