package bindly

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"

	"github.com/viant/structology"
)

// ResolutionGroupSpec is explicit opt-in authoring metadata. After lists
// successful canonical barriers; DependsOn denotes equivalent child input
// validation, not serial member scheduling.
type ResolutionGroupSpec struct {
	Name      string
	After     []string
	DependsOn []string
}

func copyGroupBinding(spec BindingSpec) BindingSpec {
	if spec.ResolutionGroup != nil {
		value := *spec.ResolutionGroup
		value.After = append([]string(nil), value.After...)
		value.DependsOn = append([]string(nil), value.DependsOn...)
		spec.ResolutionGroup = &value
	}
	return spec
}
func (p *Plan) compileResolutionGroups() error {
	grouped := false
	for _, spec := range p.bindings {
		grouped = grouped || spec.ResolutionGroup != nil
	}
	if !grouped {
		return nil
	}
	byPath := map[string]BindingSpec{}
	aliases := map[string]string{}
	for _, spec := range p.bindings {
		byPath[spec.Path] = spec
		if previous, ok := aliases[spec.Name]; spec.Name != "" && ok && previous != spec.Path {
			return fmt.Errorf("ambiguous binding alias %s", spec.Name)
		}
		if spec.Name != "" {
			aliases[spec.Name] = spec.Path
		}
	}
	canonical := func(name string) (string, error) {
		if _, ok := byPath[name]; ok {
			return name, nil
		}
		if path, ok := aliases[name]; ok {
			return path, nil
		}
		return "", fmt.Errorf("resolution group binding %s is not defined", name)
	}
	groupIndex := map[string]int{}
	for i, spec := range p.bindings {
		if spec.ResolutionGroup == nil {
			continue
		}
		spec = copyGroupBinding(spec)
		group := spec.ResolutionGroup
		if group.Name == "" || len(group.After) == 0 {
			return fmt.Errorf("binding %s requires a named resolution group and barriers", spec.Path)
		}
		if spec.When != "" || spec.Async || spec.Cacheable != nil || spec.SourceType != nil || spec.Transformer != nil {
			return fmt.Errorf("binding %s has incompatible resolution group mode", spec.Path)
		}
		if spec.Location.Kind != "path" && spec.Location.Kind != "component" {
			return fmt.Errorf("binding %s resolution group requires a path or native component", spec.Path)
		}
		seen := map[string]bool{}
		for n, name := range group.After {
			path, err := canonical(name)
			if err != nil {
				return err
			}
			if path == spec.Path || seen[path] || byPath[path].ResolutionGroup != nil {
				return fmt.Errorf("binding %s has invalid resolution barrier %s", spec.Path, name)
			}
			seen[path] = true
			group.After[n] = path
		}
		seen = map[string]bool{}
		for n, name := range group.DependsOn {
			path, err := canonical(name)
			if err != nil {
				return err
			}
			peer := byPath[path]
			if path == spec.Path || seen[path] || peer.ResolutionGroup == nil || peer.ResolutionGroup.Name != group.Name {
				return fmt.Errorf("binding %s has invalid group prerequisite %s", spec.Path, name)
			}
			seen[path] = true
			group.DependsOn[n] = path
		}
		p.bindings[i] = spec
		index, ok := groupIndex[group.Name]
		if !ok {
			index = len(p.groups)
			groupIndex[group.Name] = index
			p.groups = append(p.groups, ResolutionGroupPlan{name: group.Name, after: append([]string(nil), group.After...)})
		}
		descriptor := &p.groups[index]
		if !reflect.DeepEqual(descriptor.after, group.After) {
			return fmt.Errorf("resolution group %s has inconsistent barriers", group.Name)
		}
		descriptor.members = append(descriptor.members, spec)
	}
	// Even though member attempts are concurrent, prerequisite cycles are invalid
	// authoring and cannot establish equivalent child input validation.
	for _, group := range p.groups {
		visiting, visited := map[string]bool{}, map[string]bool{}
		var visit func(string) error
		visit = func(path string) error {
			if visiting[path] {
				return fmt.Errorf("resolution group %s prerequisite cycle at %s", group.name, path)
			}
			if visited[path] {
				return nil
			}
			visiting[path] = true
			spec := byPath[path]
			for _, dep := range spec.ResolutionGroup.DependsOn {
				canonicalPath, err := canonical(dep)
				if err != nil {
					return err
				}
				if err := visit(canonicalPath); err != nil {
					return err
				}
			}
			delete(visiting, path)
			visited[path] = true
			return nil
		}
		for _, member := range group.members {
			if err := visit(member.Path); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *invocation) bindWithGroups(ctx context.Context, target any, plan *Plan) error {
	if s.groupController == nil {
		return fmt.Errorf("resolution groups require an invocation controller")
	}
	if s.replay != nil || s.resolved != nil || s.persistent != nil || s.captureSources || len(s.allowedKinds) != 0 || len(s.delayedKinds) != 0 {
		return fmt.Errorf("resolution groups do not support replay, resolved values, explicit caches or partial binding")
	}
	completed := map[string]bool{}
	subset := func(specs []BindingSpec) *Plan {
		return &Plan{target: plan.target, fields: plan.fields, bindings: specs}
	}
	for _, group := range plan.groups {
		var barriers []BindingSpec
		for _, path := range group.after {
			if completed[path] {
				continue
			}
			for _, spec := range plan.bindings {
				if spec.Path == path {
					barriers = append(barriers, spec)
					break
				}
			}
		}
		if err := s.bind(ctx, target, subset(barriers)); err != nil {
			return err
		}
		for _, spec := range barriers {
			completed[spec.Path] = true
		}
		run, err := s.groupController.Open(ctx, group)
		if err != nil {
			return err
		}
		if run == nil {
			return fmt.Errorf("resolution group controller returned no run")
		}
		groupCtx, cancel := context.WithCancel(ctx)
		var collector sync.Mutex
		var observation sync.Mutex
		var publication sync.Mutex
		var wait sync.WaitGroup
		failures := []*BindingError{}
		var terminal error
		start := make(chan struct{})
		for _, member := range group.members {
			member := member
			wait.Add(1)
			go func() {
				defer wait.Done()
				<-start
				outcome := ResolutionOutcome{path: member.Path}
				var workerCtx context.Context
				var workerErr error
				defer func() {
					if panicValue := recover(); panicValue != nil {
						if actual, ok := panicValue.(error); ok {
							workerErr = fmt.Errorf("resolution group binding panic: %w", actual)
						} else {
							workerErr = fmt.Errorf("resolution group binding panic: %v", panicValue)
						}
						outcome.terminal = true
						cancel()
					}
					outcome.cause = workerErr
					if workerErr != nil && outcome.attempted {
						var actual *BindingError
						if !errors.As(workerErr, &actual) || actual.Path != member.Path {
							actual = &BindingError{Path: member.Path, Name: member.Name, Kind: member.Location.Kind, In: member.Location.In, Code: member.ErrorCode, Message: member.ErrorMessage, Cause: workerErr}
						}
						outcome.failure = copyBindingError(actual)
					}
					// Append before the observer/controller call: this is binding completion,
					// rather than SQL retirement or a callback's completion order.
					collector.Lock()
					if outcome.failure != nil {
						failures = append(failures, copyBindingError(outcome.failure))
					}
					collector.Unlock()
					if workerCtx == nil {
						workerCtx = groupCtx
					}
					observation.Lock()
					observeErr := run.Observe(workerCtx, outcome)
					observation.Unlock()
					if observeErr != nil {
						collector.Lock()
						terminal = errors.Join(terminal, observeErr)
						collector.Unlock()
						cancel()
					}
				}()
				workerCtx, workerErr = run.Enter(groupCtx, member.Path)
				if workerErr != nil {
					return
				}
				outcome.attempted = true
				// Each worker owns its scratch record and resolver maps. Snapshot source
				// lookups cannot race canonical field/marker publication.
				publication.Lock()
				scratch := reflect.New(plan.target)
				scratch.Elem().Set(reflect.ValueOf(target).Elem())
				source := s.groupSourceSnapshot()
				publication.Unlock()
				worker := *s
				worker.active = map[string]bool{}
				worker.cache = map[resolutionKey]resolution{}
				worker.persistent = nil
				worker.observer = func(_ context.Context, event BindingEvent) error {
					if event.Target == scratch.Interface() && event.Path == member.Path {
						outcome.value, outcome.found, outcome.metadata = event.Value, true, event.Metadata
					}
					return nil
				}
				worker.source = source
				worker.groupSource = func() *structology.State {
					publication.Lock()
					defer publication.Unlock()
					return s.groupSourceSnapshot()
				}
				worker.groupController = nil
				spec := member
				spec.ResolutionGroup = nil
				spec.MarkerField = ""
				workerErr = worker.bind(workerCtx, scratch.Interface(), subset([]BindingSpec{spec}))
				if workerErr != nil {
					return
				}
				if !outcome.found {
					return
				}
				value, found := plan.fields[member.Path].Value(scratch)
				if !found || !value.CanInterface() {
					workerErr = fmt.Errorf("assigned group field is not accessible")
					return
				}
				outcome.value, outcome.found = value.Interface(), true
				publication.Lock()
				workerErr = plan.fields[member.Path].Set(reflect.ValueOf(target), outcome.value)
				if workerErr == nil && member.MarkerField != "" {
					workerErr = plan.fields[member.MarkerField].Set(reflect.ValueOf(target), true)
				}
				for key, value := range worker.cache {
					s.cache[key] = value
				}
				publication.Unlock()
				if workerErr != nil {
					return
				}
				if s.observer != nil {
					observation.Lock()
					workerErr = s.observed(workerCtx, target, member, resolution{value: outcome.value, found: true, metadata: outcome.metadata})
					if workerErr != nil {
						outcome.terminal = true
						cancel()
					}
					observation.Unlock()
				}
			}()
		}
		close(start)
		wait.Wait()
		cancel()
		closeErr := run.Close(context.WithoutCancel(ctx))
		if closeErr != nil {
			terminal = errors.Join(terminal, closeErr)
		}
		if len(failures) != 0 {
			return errors.Join(&ResolutionGroupError{failures: failures}, terminal)
		}
		if terminal != nil {
			return terminal
		}
		for _, member := range group.members {
			completed[member.Path] = true
		}
	}
	var remaining []BindingSpec
	for _, spec := range plan.bindings {
		if !completed[spec.Path] {
			remaining = append(remaining, spec)
		}
	}
	return s.bind(ctx, target, subset(remaining))
}

func (s *invocation) groupSourceSnapshot() *structology.State {
	if s.source == nil {
		return nil
	}
	// WithSource(input) remains a lookup snapshot, while observed Target and
	// assigned pointers remain the original destination and values.
	sourceValue := s.source.State()
	value := reflect.ValueOf(sourceValue)
	if value.Kind() == reflect.Pointer && !value.IsNil() && value.Elem().Kind() == reflect.Struct {
		snapshot := reflect.New(value.Elem().Type())
		snapshot.Elem().Set(value.Elem())
		for _, field := range reflect.VisibleFields(value.Elem().Type()) {
			if field.Tag.Get("setMarker") == "true" {
				marker := snapshot.Elem().FieldByIndex(field.Index)
				if marker.Kind() == reflect.Pointer && !marker.IsNil() && marker.Elem().Kind() == reflect.Struct {
					private := reflect.New(marker.Elem().Type())
					private.Elem().Set(marker.Elem())
					marker.Set(private)
				}
			}
		}
		return structology.NewStateType(snapshot.Type()).WithValue(snapshot.Interface())
	}
	return s.source
}
