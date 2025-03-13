package locator

import "context"

type Locator interface {
	Value(ctx context.Context, name string) (interface{}, bool, error)
	Kind() string
}
