package bindly

import (
	"github.com/viant/bindly/resource"
	"io/fs"
)

func WithResources(store *resource.Store) InjectorOption {
	return func(i *Injector) {
		if store != nil {
			i.resources = store
		}
	}
}
func WithResourceFS(name string, source fs.FS) InjectorOption {
	return func(i *Injector) {
		if err := i.resources.Register(name, source); err != nil {
			i.initErr = err
		}
	}
}
func (i *Injector) Resources() *resource.Store           { return i.resources }
func (i *Injector) ResourceFS(name string) (fs.FS, bool) { return i.resources.Lookup(name) }
func (i *Injector) ReadResource(reference string) ([]byte, error) {
	return i.resources.ReadFile(reference)
}
