package locator

import (
	"context"
	"reflect"
)

type Locator interface {
	Value(ctx context.Context, targetType reflect.Type, name string) (interface{}, bool, error)
	Kind() string
}
