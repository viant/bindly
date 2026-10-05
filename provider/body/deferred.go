package body

import (
	"context"
	"fmt"
	"sync"
)

// NewDeferred preserves the body provider API while initializing bytes and MIME
// metadata only when a value or replay source is requested. Initialization errors
// are retained; decoding still happens independently for each target and field.
func NewDeferred(load func(context.Context) (*Source, error)) *Source {
	return &Source{deferred: &deferredSource{load: load}}
}

type deferredSource struct {
	once   sync.Once
	load   func(context.Context) (*Source, error)
	source *Source
	err    error
}

func (s *Source) resolve(ctx context.Context) (*Source, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	d := s.deferred
	d.once.Do(func() {
		if d.load == nil {
			d.err = fmt.Errorf("deferred body source loader is required")
			return
		}
		d.source, d.err = d.load(ctx)
		if d.err == nil && (d.source == nil || d.source.deferred != nil) {
			d.err = fmt.Errorf("deferred body loader must return an initialized source")
		}
	})
	return d.source, d.err
}
