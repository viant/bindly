package locator

import "github.com/viant/structology"

type Provider interface {
	Locate(state *structology.State) Locator
	Kind() string
	Priority() int
}
