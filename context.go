package bindly

import (
	"github.com/viant/structology"
	"reflect"
)

// BindingContext represents binding context with state
type BindingContext[T any] struct {
	injector   *Injector
	state      *structology.State
	bindings   []Bindings
	valueCache *ValueCache
}

func WithState[T any](binder *Injector, state interface{}, opt ...BindingOption[T]) *BindingContext[T] {
	reflectType := reflect.TypeOf(state)
	structType, ok := binder.structTypeCache.Get(reflectType)
	if !ok {
		structType = structology.NewStateType(reflectType)
		binder.structTypeCache.Put(reflectType, structType)
	}
	stateValue := structType.WithValue(state)
	ret := &BindingContext[T]{injector: binder, state: stateValue, valueCache: NewValueCache()}
	for _, o := range opt {
		o(ret)
	}
	return ret
}
