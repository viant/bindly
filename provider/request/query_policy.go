package request

import "context"

// QueryPolicy is invocation-local query presence policy.
type QueryPolicy struct{ IgnoreEmptyParameters bool }
type queryPolicyKey struct{}

// WithQueryPolicy overrides scope defaults for this invocation only.
func WithQueryPolicy(ctx context.Context, policy QueryPolicy) context.Context {
	return context.WithValue(ctx, queryPolicyKey{}, policy)
}
