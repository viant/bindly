package bindly

import (
	"context"
	"fmt"
	"github.com/viant/bindly/state"
	"github.com/viant/structology"
	"reflect"
)

type Bindings []*Binding
type BindingType struct {
	Bindings []Bindings
	Type     *structology.StateType
}

func (b *Injector) buildBindings(ctx context.Context, destState *structology.StateType) (*BindingType, error) {
	rootSelector := destState.RootSelectors()
	if len(rootSelector) == 0 {
		return nil, fmt.Errorf("invalid type: %s", destState.Type().String())
	}
	var bindings Bindings
	for i, selector := range rootSelector {
		tag := selector.Tag()
		tagName, ok, err := resolveBindingTag(tag, b.bindingTag)
		if err != nil {
			return nil, fmt.Errorf("parse binding for %s: %w", selector.Path(), err)
		}
		aBinding := &Binding{location: &state.Location{}, selector: rootSelector[i], Tag: tag}
		if !ok {
			if selector.Type().Kind() == reflect.Interface {
				aBinding.location.In = selector.Type().String()
				aBinding.location.Kind = b.interfaceKind
				bindings = append(bindings, aBinding)
			}
			continue
		}
		if err := b.extractTransformer(ctx, aBinding); err != nil {
			return nil, err
		}

		if err := b.extractBinding(aBinding, tagName); err != nil {
			return nil, fmt.Errorf("parse binding for %s: %w", selector.Path(), err)
		}
		if aBinding.location.Kind == "" && aBinding.location.In == "" {
			return nil, fmt.Errorf("binding location was empty for: %v", selector.Path())
		}

		bindings = append(bindings, aBinding)
	}
	return &BindingType{
		Bindings: groupBindings(bindings),
		Type:     destState,
	}, nil
}
