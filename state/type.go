package state

import (
	"github.com/viant/bindly/types"
	"github.com/viant/structology"
)

type Type struct {
	StateType structology.StateType

	Schema types.Type
}

func (t *Type) Init() {
	selectors := t.StateType.RootSelectors()
	for _, selector := range selectors {
		selector.Path()
		selector.Tag()
	}
}
