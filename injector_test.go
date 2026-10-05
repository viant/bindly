package bindly_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/viant/bindly"
	"github.com/viant/bindly/locator/buildin"
	"github.com/viant/structology"
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
		Bar     *Bar `bind:"kind=instance,in=bar"`
		Counter ICounter
		Key1    string `bind:"kind=state,in=Session.Key1"`
	}
	foo := &Foo{}

	var opts = append([]bindly.InjectorOption{}, bindly.WithProviders(
		buildin.Struct("state", "", 1),
		buildin.Map("instance", "Instances", 1),
		buildin.Map("interface", "Interfaces", 1)))

	injector, err := bindly.NewInjector(opts...)
	assert.NoError(t, err)
	err = bindly.WithState[Foo](injector, dependencies).Inject(context.Background(), foo)

	assert.NoError(t, err)

	// verify that scalar state and instance bindings were injected
	assert.Equal(t, "abc", foo.Key1) // explicit state binding via tag
	if assert.NotNil(t, foo.Bar) {   // instance binding from Instances["bar"]
		assert.Equal(t, "attr1", foo.Bar.Attr1)
	}

	// verify that interface binding resolved to the configured Counter instance
	if assert.NotNil(t, foo.Counter) {
		foo.Counter.Inc()
		concrete, ok := foo.Counter.(*Counter)
		if assert.True(t, ok, "expected *Counter implementation") {
			assert.Equal(t, 1, concrete.count)
		}
	}
}
