package bindly

func WithValueCache(cache *ValueCache) BindOption { return func(o *bindOptions) { o.cache = cache } }
func OnlyKinds(kinds ...string) BindOption {
	return func(o *bindOptions) { o.allowedKinds = kindSet(kinds) }
}
func SkipKinds(kinds ...string) BindOption {
	return func(o *bindOptions) { o.delayedKinds = kindSet(kinds) }
}
func AllowMissingRequired() BindOption { return func(o *bindOptions) { o.strictMissing = false } }
func kindSet(kinds []string) map[string]bool {
	result := make(map[string]bool, len(kinds))
	for _, kind := range kinds {
		result[kind] = true
	}
	return result
}
