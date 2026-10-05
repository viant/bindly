package bindly

import (
	"github.com/viant/bindly/internal"
	"github.com/viant/structology"
	"reflect"
	"sync"
)

type ValueCache struct {
	internal.Map[string, interface{}]
	locker internal.Map[string, sync.Locker]
}

func (c *ValueCache) lock(key string) sync.Locker {
	locker, _ := c.locker.LoadOrStore(key, sync.Locker(&sync.Mutex{}))
	return locker
}

// Clear empties the cache
func (c *ValueCache) Clear() {
	c.Map.Clear()
	c.locker.Clear()
}

func NewValueCache() *ValueCache {
	return &ValueCache{Map: internal.NewMap[string, interface{}](), locker: internal.NewMap[string, sync.Locker]()}
}

type BindingCache struct {
	internal.Map[reflect.Type, *BindingType]
}

func NewBindingCache() *BindingCache {
	return &BindingCache{Map: internal.NewMap[reflect.Type, *BindingType]()}
}

type StructTypeCache struct {
	internal.Map[reflect.Type, *structology.StateType]
}

func NewStructTypeCache() *StructTypeCache {
	return &StructTypeCache{Map: internal.NewMap[reflect.Type, *structology.StateType]()}
}
