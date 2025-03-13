package bindly_test

import (
	"context"
	"github.com/stretchr/testify/assert"
	"github.com/viant/bindly"
	"github.com/viant/bindly/locator/buildin"
	"github.com/viant/structology"
	"strings"
	"testing"
)

type ICounter interface {
	Inc()
}

type Counter struct {
	count int
}

func (d *Counter) Inc() {
	d.count += 1
}

func TestInjector_Inject(t *testing.T) {
	type SessionHas struct {
		ID   bool
		Key1 bool
		Key2 bool
	}
	type Session struct {
		ID   int
		Key1 string
		Key2 string
		Has  *SessionHas `setMarker:"true"`
	}

	type Bar struct {
		Attr1 string
		Body  strings.Reader
	}

	type DependencySetup struct {
		Interfaces map[string]interface{}
		Instances  map[string]interface{}
		Session    *Session
	}

	var iCounter ICounter

	dependencies := &DependencySetup{
		Session: &Session{
			ID:   101,
			Key1: "abc",
			Has: &SessionHas{
				ID:   true,
				Key1: true,
			},
		},
		Interfaces: map[string]interface{}{
			structology.InterfaceTypeOf(&iCounter).String(): &Counter{},
		},
		Instances: map[string]interface{}{
			"bar": &Bar{
				Attr1: "attr1",
			},
		},
	}
	type Foo struct {
		ID      int
		Bar     *Bar `bind:"kind=instance,in=bar"`
		Counter ICounter
		Key1    string `bind:"kind=state,in=Session.Key1"`
	}
	foo := &Foo{}

	var opts = append([]bindly.InjectorOption{}, bindly.WithProviders(
		buildin.Struct("state", "", 1),
		buildin.Map("instance", "Instances", 1),
		buildin.Map("interface", "Interfaces", 1)))

	injector := bindly.NewInjector(opts...)
	err := bindly.WithState[Foo](injector, dependencies).Inject(context.Background(), foo)
	if err != nil {
		return
	}
	assert.Nil(t, err)

}
