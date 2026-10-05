package state

import (
	"reflect"
	"testing"

	"github.com/viant/bindly/types"
	"github.com/viant/structology"
)

// TestType_Init ensures that Init iterates over root selectors without panicking
// and that Type can be safely used with a simple schema and Go struct.
func TestType_Init(t *testing.T) {
	type Sample struct {
		Field string `bind:"kind=state,in=Field"`
	}

	// NewStateType expects a reflect.Type, not a value.
	stateType := structology.NewStateType(reflect.TypeOf(Sample{}))
	schemaType := types.NewType(stateType.Type())

	typeInstance := &Type{
		StateType: *stateType,
		Schema:    *schemaType,
	}

	// Init should simply walk selectors; this test ensures it does not panic.
	typeInstance.Init()
}
