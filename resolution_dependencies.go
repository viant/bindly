package bindly

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
)

// ResolutionPrerequisiteError describes a skipped member, not an attempted read.
type ResolutionPrerequisiteError struct {
	Path         string
	Prerequisite string
	Cause        error
}

func (e *ResolutionPrerequisiteError) Error() string {
	return fmt.Sprintf("resolution group member %s skipped: prerequisite %s failed: %v", e.Path, e.Prerequisite, e.Cause)
}
func (e *ResolutionPrerequisiteError) Unwrap() error { return e.Cause }

type dependencyResult struct {
	index      int
	ctx        context.Context
	outcome    ResolutionOutcome
	observeErr error
}

func (s *invocation) bindDependencyGroup(ctx context.Context, target any, plan *Plan, group ResolutionGroupPlan, run ResolutionGroupRun) (err error) {
	// Open has succeeded: even admission failure must close its controller run.
	defer func() { err = errors.Join(err, run.Close(context.WithoutCancel(ctx))) }()
	scheduler := resolutionSchedulerFrom(ctx)
	if scheduler == nil {
		limit := s.groupLimit
		if limit == 0 {
			limit = 4
		}
		scheduler = &resolutionScheduler{slots: make(chan struct{}, limit)}
		ctx = context.WithValue(ctx, resolutionSchedulerKey{}, scheduler)
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var publication, observation sync.Mutex
	results := make([]dependencyResult, len(group.members))
	status := make([]uint8, len(group.members)) // pending, running, completed
	byPath := make(map[string]int, len(group.members))
	for i, member := range group.members {
		byPath[member.Path] = i
	}
	done := make(chan dependencyResult, len(group.members))
	completed, running := 0, 0
	observe := func(result *dependencyResult) {
		observationCtx := result.ctx
		if observationCtx == nil {
			observationCtx = ctx
		}
		result.observeErr = observeDependencyOutcome(observationCtx, run, result.outcome, &observation)
		if result.observeErr != nil || result.outcome.terminal {
			cancel()
		}
	}
	for completed < len(group.members) {
		before := completed
		ready := -1
		for i, member := range group.members {
			if status[i] != 0 {
				continue
			}
			var skip error
			waiting := false
			for _, path := range member.ResolutionGroup.DependsOn {
				prerequisite := byPath[path]
				if status[prerequisite] != 2 {
					waiting = true
					continue
				}
				prior := results[prerequisite]
				if cause := errors.Join(prior.outcome.cause, prior.observeErr); cause != nil {
					skip = &ResolutionPrerequisiteError{Path: member.Path, Prerequisite: path, Cause: cause}
					break
				}
			}
			if ctx.Err() != nil {
				skip = context.Cause(ctx)
			}
			if skip != nil {
				result := dependencyResult{index: i, outcome: ResolutionOutcome{path: member.Path, cause: skip}}
				observe(&result)
				results[i], status[i] = result, 2
				completed++
				continue
			}
			if !waiting && ready == -1 {
				ready = i
			}
		}
		if completed == len(group.members) {
			break
		}
		if completed != before && ready == -1 {
			continue
		}
		var slots chan struct{}
		if ready >= 0 && ctx.Err() == nil {
			slots = scheduler.slots
		}
		// Admission happens before spawning: pending members consume no goroutine.
		// Explicitly suspended ancestors retain only their waiting call stacks.
		select {
		case slots <- struct{}{}:
			if ctx.Err() != nil {
				<-slots
				continue
			}
			index := ready
			status[index] = 1
			running++
			go func() {
				lease := &resolutionLease{scheduler: scheduler, held: true}
				workerCtx := context.WithValue(ctx, resolutionLeaseKey{}, lease)
				result := s.executeDependencyMember(workerCtx, target, plan, group.members[index], run, &publication)
				result.index = index
				observe(&result)
				lease.release()
				done <- result
			}()
		case result := <-done:
			results[result.index], status[result.index] = result, 2
			running--
			completed++
		case <-ctx.Done():
			// Join started work without spinning on the canceled context.
			for running > 0 {
				result := <-done
				results[result.index], status[result.index] = result, 2
				running--
				completed++
			}
		}
	}
	var failures []*BindingError
	var terminal []error
	for _, result := range results {
		if result.outcome.failure != nil {
			failures = append(failures, copyBindingError(result.outcome.failure))
		} else if result.outcome.cause != nil {
			var skipped *ResolutionPrerequisiteError
			if !errors.As(result.outcome.cause, &skipped) {
				terminal = append(terminal, result.outcome.cause)
			}
		}
		terminal = append(terminal, result.observeErr)
	}
	if len(failures) > 0 {
		terminal = append([]error{&ResolutionGroupError{failures: failures}}, terminal...)
	}
	// A panic cancels peers even when a permissive controller returns nil.
	terminal = append(terminal, context.Cause(ctx))
	return errors.Join(terminal...)
}

func observeDependencyOutcome(ctx context.Context, run ResolutionGroupRun, outcome ResolutionOutcome, mu *sync.Mutex) (err error) {
	mu.Lock()
	defer mu.Unlock()
	defer func() {
		if value := recover(); value != nil {
			err = dependencyPanic(value)
		}
	}()
	return run.Observe(context.WithoutCancel(ctx), outcome)
}

func dependencyPanic(value any) error {
	if cause, ok := value.(error); ok {
		return fmt.Errorf("resolution group binding panic: %w", cause)
	}
	return fmt.Errorf("resolution group binding panic: %v", value)
}

func (s *invocation) executeDependencyMember(ctx context.Context, target any, plan *Plan, member BindingSpec, run ResolutionGroupRun, publication *sync.Mutex) (result dependencyResult) {
	result.outcome.path = member.Path
	var workerErr error
	defer func() {
		if value := recover(); value != nil {
			workerErr = dependencyPanic(value)
			result.outcome.terminal = true
		}
		result.outcome.cause = workerErr
		if workerErr != nil && result.outcome.attempted {
			result.outcome.failure = &BindingError{Path: member.Path, Name: member.Name, Kind: member.Location.Kind, In: member.Location.In, Code: member.ErrorCode, Message: member.ErrorMessage, Cause: workerErr}
			var bindingErr *BindingError
			if errors.As(workerErr, &bindingErr) && bindingErr.Path == member.Path {
				result.outcome.failure = copyBindingError(bindingErr)
			}
		}
	}()
	workerCtx, enterErr := run.Enter(ctx, member.Path)
	result.ctx = workerCtx
	workerErr = enterErr
	if workerErr != nil {
		return result
	}
	if workerCtx == nil {
		workerErr = fmt.Errorf("resolution group controller returned no member context")
		return result
	}
	if resolutionSchedulerFrom(workerCtx) != resolutionSchedulerFrom(ctx) || workerCtx.Value(resolutionLeaseKey{}) != ctx.Value(resolutionLeaseKey{}) {
		workerErr = fmt.Errorf("resolution group controller must retain the invocation scheduler context")
		return result
	}
	// Controller context decoration must not detach a member from group cancellation.
	workerCtx, cancel := context.WithCancel(workerCtx)
	stop := context.AfterFunc(ctx, cancel)
	defer func() { stop(); cancel() }()
	result.ctx = workerCtx
	result.outcome.attempted = true
	worker := *s
	var scratch reflect.Value
	func() {
		publication.Lock()
		defer publication.Unlock()
		scratch = reflect.New(plan.target)
		scratch.Elem().Set(reflect.ValueOf(target).Elem())
		detachDependencyPath(scratch.Elem(), plan.fields[member.Path].Index)
		worker.source = s.groupSourceSnapshot()
	}()
	worker.active = map[string]bool{}
	worker.cache = map[resolutionKey]resolution{}
	worker.persistent, worker.groupController, worker.groupSource = nil, nil, nil
	worker.observer = func(_ context.Context, event BindingEvent) error {
		if event.Target == scratch.Interface() && event.Path == member.Path {
			result.outcome.value, result.outcome.found, result.outcome.metadata = event.Value, true, event.Metadata
		}
		return nil
	}
	spec := member
	spec.ResolutionGroup, spec.MarkerField = nil, ""
	workerErr = worker.bind(workerCtx, scratch.Interface(), &Plan{target: plan.target, fields: plan.fields, bindings: []BindingSpec{spec}})
	if workerErr != nil || !result.outcome.found {
		return result
	}
	value, found := plan.fields[member.Path].Value(scratch)
	if !found || !value.CanInterface() {
		workerErr = fmt.Errorf("assigned group field is not accessible")
		return result
	}
	result.outcome.value = value.Interface()
	publication.Lock()
	defer publication.Unlock()
	// Published pointer containers must not mutate snapshots retained by peers.
	detachDependencyPath(reflect.ValueOf(target).Elem(), plan.fields[member.Path].Index)
	workerErr = plan.fields[member.Path].Set(reflect.ValueOf(target), result.outcome.value)
	if workerErr == nil && member.MarkerField != "" {
		detachDependencyPath(reflect.ValueOf(target).Elem(), plan.fields[member.MarkerField].Index)
		workerErr = plan.fields[member.MarkerField].Set(reflect.ValueOf(target), true)
	}
	if workerErr == nil && s.observer != nil {
		workerErr = s.observed(workerCtx, target, member, resolution{value: result.outcome.value, found: true, metadata: result.outcome.metadata})
		if workerErr != nil {
			result.outcome.terminal = true
		}
	}
	return result
}

// Copy only pointer ancestors of the destination, not completed child results
// or other potentially large input values. Providers treat source values as read-only.
func detachDependencyPath(value reflect.Value, indexes []int) {
	for i, index := range indexes {
		for value.Kind() == reflect.Pointer {
			if value.IsNil() {
				return
			}
			copy := reflect.New(value.Type().Elem())
			copy.Elem().Set(value.Elem())
			value.Set(copy)
			value = value.Elem()
		}
		if i == len(indexes)-1 {
			return
		}
		value = value.Field(index)
	}
}
