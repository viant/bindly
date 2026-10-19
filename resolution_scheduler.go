package bindly

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// WithResolutionGroupConcurrency bounds runnable dependency-group members across
// nested binds in one invocation. Zero uses four; negative values are invalid.
// It does not change legacy concurrent groups or ordinary sequential binding.
func WithResolutionGroupConcurrency(limit int) BindOption {
	return func(o *bindOptions) { o.groupLimit = limit }
}

type resolutionSchedulerKey struct{}
type resolutionLeaseKey struct{}
type resolutionScheduler struct{ slots chan struct{} }
type resolutionLease struct {
	mu        sync.Mutex
	scheduler *resolutionScheduler
	held      bool
}

func resolutionSchedulerFrom(ctx context.Context) *resolutionScheduler {
	scheduler, _ := ctx.Value(resolutionSchedulerKey{}).(*resolutionScheduler)
	return scheduler
}

func (l *resolutionLease) release() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.held {
		l.held = false
		<-l.scheduler.slots
	}
}

// RunResolutionGroupPhase executes runnable work under the inherited dependency
// group's limit. Use it for sequential descendant work inside a suspended wait;
// explicitly grouped descendants acquire their own permits automatically.
// Existing member phases reuse their permit. No scheduler means ordinary execution.
func RunResolutionGroupPhase(ctx context.Context, execute func(context.Context) error) error {
	if execute == nil {
		return fmt.Errorf("resolution group phase is required")
	}
	if lease, _ := ctx.Value(resolutionLeaseKey{}).(*resolutionLease); lease != nil {
		lease.mu.Lock()
		held := lease.held
		lease.mu.Unlock()
		if !held {
			return fmt.Errorf("resolution group member is suspended")
		}
		return execute(ctx)
	}
	scheduler := resolutionSchedulerFrom(ctx)
	if scheduler == nil {
		return execute(ctx)
	}
	if ctx.Err() != nil {
		return context.Cause(ctx)
	}
	select {
	case scheduler.slots <- struct{}{}:
		lease := &resolutionLease{scheduler: scheduler, held: true}
		defer lease.release()
		if ctx.Err() != nil {
			return context.Cause(ctx)
		}
		return execute(context.WithValue(ctx, resolutionLeaseKey{}, lease))
	case <-ctx.Done():
		return context.Cause(ctx)
	}
}

// SuspendResolutionGroup releases the current runnable permit while wait waits
// for descendants. Nested binds inherit the same scheduler, but not the parent's
// permit. On return the permit is reacquired before caller execution resumes.
//
// Call only around dependency waits, not database work or arbitrary execution.
// A member context must not be shared by concurrently executing branches. A
// canceled wait returns without reacquiring; the caller must propagate the error.
// Outside a dependency-group member this simply calls wait.
func SuspendResolutionGroup(ctx context.Context, wait func(context.Context) error) error {
	if wait == nil {
		return fmt.Errorf("resolution group wait is required")
	}
	lease, _ := ctx.Value(resolutionLeaseKey{}).(*resolutionLease)
	if lease == nil {
		return wait(ctx)
	}
	lease.mu.Lock()
	if !lease.held {
		lease.mu.Unlock()
		return fmt.Errorf("resolution group member is already suspended")
	}
	lease.held = false
	<-lease.scheduler.slots
	lease.mu.Unlock()
	err := wait(context.WithValue(ctx, resolutionLeaseKey{}, (*resolutionLease)(nil)))
	if ctx.Err() != nil {
		return errors.Join(err, context.Cause(ctx))
	}
	select {
	case lease.scheduler.slots <- struct{}{}:
		lease.mu.Lock()
		lease.held = true
		lease.mu.Unlock()
		return err
	case <-ctx.Done():
		return errors.Join(err, context.Cause(ctx))
	}
}
