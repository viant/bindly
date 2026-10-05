package bindly

import (
	"context"
	"reflect"

	"github.com/viant/bindly/locator"
	"github.com/viant/structology"
)

type dummyLocator struct{}

func (d *dummyLocator) Value(_ context.Context, _ reflect.Type, _ string) (interface{}, bool, error) {
	return nil, false, nil
}

func (d *dummyLocator) Kind() string {
	return "test"
}

type testProvider struct {
	kind     string
	priority int
	loc      locator.Locator
}

func (p *testProvider) Locate(_ *structology.State) locator.Locator {
	return p.loc
}

func (p *testProvider) Kind() string {
	return p.kind
}

func (p *testProvider) Priority() int {
	return p.priority
}
