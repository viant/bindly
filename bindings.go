package bindly

import (
	"context"
	"embed"
	"fmt"
	"github.com/viant/bindly/locator"
	"github.com/viant/bindly/state"
	"github.com/viant/structology"
	"reflect"
	"sort"
)

type Bindings []*Binding
type BindingType struct {
	Bindings []Bindings
	Type     *structology.StateType
}

func (b Bindings) GroupByPriority(registry *locator.Registry) ([]Bindings, error) {
	// Assign providers based on location kind
	for _, binding := range b {
		provider, ok := registry.Lookup(binding.location.Kind)
		if !ok {
			return nil, fmt.Errorf("failed to lookup binding provider for: %v, path: %v", binding.location.Kind, binding.selector.Path())
		}
		binding.provider = provider
	}

	// Sort bindings by provider priority
	sort.Slice(b, func(i, j int) bool {
		return b[i].provider.Priority() < b[j].provider.Priority()
	})

	// Group by priority
	var result []Bindings
	if len(b) == 0 {
		return result, nil
	}

	// Initialize first group
	currentGroup := Bindings{b[0]}
	currentPriority := b[0].provider.Priority()

	for i := 1; i < len(b); i++ {
		binding := b[i]
		if binding.provider.Priority() != currentPriority {
			// Store previous group and start a new one
			result = append(result, currentGroup)
			currentGroup = Bindings{}
			currentPriority = binding.provider.Priority()
		}
		currentGroup = append(currentGroup, binding)
	}

	// Append the last group
	result = append(result, currentGroup)
	return result, nil
}

func (b *Injector) buildBindings(ctx context.Context, destState *structology.StateType) (*BindingType, error) {
	rootSelector := destState.RootSelectors()
	if len(rootSelector) == 0 {
		return nil, fmt.Errorf("invalid type: %s", destState.Type().String())
	}
	var embedFs *embed.FS
	if b.embedder != nil {
		embedFs = b.embedder.EmbedFS()
	}
	var bindings Bindings
	for i, selector := range rootSelector {
		tag := selector.Tag()
		_, ok := tag.Lookup(b.bindingTag)
		aBinding := &Binding{location: &state.Location{}, selector: rootSelector[i]}
		if !ok {
			if selector.Type().Kind() == reflect.Interface {
				aBinding.location.In = selector.Type().String()
				aBinding.location.Kind = b.interfaceKind
				bindings = append(bindings, aBinding)
			}
			continue
		}
		if err := b.extractTransformer(ctx, aBinding, embedFs); err != nil {
			return nil, err
		}

		b.extractBinding(aBinding)
		if aBinding.location.Kind == "" && aBinding.location.In == "" {
			return nil, fmt.Errorf("binding location was empty for: %v", selector.Path())
		}
		bindings = append(bindings, aBinding)
	}
	groups, err := bindings.GroupByPriority(b.locators)
	if err != nil {
		return nil, err
	}
	return &BindingType{
		Bindings: groups,
		Type:     destState,
	}, nil
}
