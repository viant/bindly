package bindly

import "context"

// ResolutionGroupController authorizes a finite compiled group for one bind.
// A controller is invocation scoped; metadata alone grants no continuation.
type ResolutionGroupController interface {
	Open(context.Context, ResolutionGroupPlan) (ResolutionGroupRun, error)
}
type ResolutionGroupRun interface {
	Enter(context.Context, string) (context.Context, error)
	Observe(context.Context, ResolutionOutcome) error
	Close(context.Context) error
}

func WithResolutionGroupController(controller ResolutionGroupController) BindOption {
	return func(o *bindOptions) { o.groupController = controller }
}

// ResolutionGroupPlan exposes copied immutable canonical descriptors.
type ResolutionGroupPlan struct {
	name    string
	after   []string
	members []BindingSpec
}

func (p ResolutionGroupPlan) Name() string    { return p.name }
func (p ResolutionGroupPlan) After() []string { return append([]string(nil), p.after...) }
func (p ResolutionGroupPlan) Members() []BindingSpec {
	result := make([]BindingSpec, len(p.members))
	for i, spec := range p.members {
		result[i] = copyGroupBinding(spec)
		// Controllers receive detached canonical descriptors, not extension,
		// transformer or default-value ownership retained by the compiled plan.
		result[i].Extension, result[i].DefaultValue, result[i].Transformer = nil, nil, nil
		for _, flag := range []**bool{&result[i].Required, &result[i].Cacheable} {
			if *flag != nil {
				value := **flag
				*flag = &value
			}
		}
		for _, count := range []**int{&result[i].MinAllowedRecords, &result[i].MaxAllowedRecords, &result[i].ExpectedReturned} {
			if *count != nil {
				value := **count
				*count = &value
			}
		}
	}
	return result
}

// ResolutionOutcome describes the actual completed binding, including an
// actual nested BindingError. It never retires child activities.
type ResolutionOutcome struct {
	attempted bool
	cause     error
	terminal  bool
	path      string
	failure   *BindingError
	value     any
	found     bool
	metadata  any
}

func (o ResolutionOutcome) Path() string           { return o.path }
func (o ResolutionOutcome) Failure() *BindingError { return copyBindingError(o.failure) }
func (o ResolutionOutcome) Value() (any, bool)     { return o.value, o.found }
func (o ResolutionOutcome) Metadata() any          { return o.metadata }

// Terminal reports an actual observer or panic-stage failure, never author metadata.
func (o ResolutionOutcome) Terminal() bool { return o.terminal }

// Attempted distinguishes actual canonical binding from denied admission.
func (o ResolutionOutcome) Attempted() bool { return o.attempted }
func (o ResolutionOutcome) Cause() error    { return o.cause }
