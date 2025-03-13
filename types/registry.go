package types

import "github.com/viant/bindly/internal"

var registry = internal.NewMap[string, *Type]()

func RegisterType(t *Type) {
	registry.Put(t.Name, t)
}

func LookupType(name string) (*Type, bool) {
	return registry.Get(name)
}
