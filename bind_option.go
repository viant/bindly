package bindly

type bindOptions struct {
	plan          *Plan
	source        interface{}
	cache         *ValueCache
	allowedKinds  map[string]bool
	delayedKinds  map[string]bool
	strictMissing bool
}

// BindOption configures one non-generic binding operation.
type BindOption func(*bindOptions)

func WithPlan(plan *Plan) BindOption {
	return func(options *bindOptions) {
		options.plan = plan
	}
}

// WithSource supplies the state consumed by state-backed providers. Providers
// that close over request-scoped data may ignore it.
func WithSource(source interface{}) BindOption {
	return func(options *bindOptions) {
		options.source = source
	}
}

func WithValueCache(cache *ValueCache) BindOption {
	return func(options *bindOptions) {
		options.cache = cache
	}
}

func OnlyKinds(kinds ...string) BindOption {
	return func(options *bindOptions) {
		options.allowedKinds = kindSet(kinds)
	}
}

func SkipKinds(kinds ...string) BindOption {
	return func(options *bindOptions) {
		options.delayedKinds = kindSet(kinds)
	}
}

func AllowMissingRequired() BindOption {
	return func(options *bindOptions) {
		options.strictMissing = false
	}
}

func kindSet(kinds []string) map[string]bool {
	result := make(map[string]bool, len(kinds))
	for _, kind := range kinds {
		result[kind] = true
	}
	return result
}
