package locator

// State is a minimal stand-in used only by tests that construct
// mockProvider implementations. The production code in this package
// does not require a concrete State type, but tests refer to it in
// the Provider interface.
//
// Keeping it here avoids leaking test-only types into other
// packages while satisfying the compiler.
type State struct{}

