package locator

// Provider priority bands make dependency order explicit without coupling
// consumers to provider-specific numeric values.
const (
	PrioritySource = 100 * (iota + 1)
	PriorityTransform
	PriorityDependent
)
