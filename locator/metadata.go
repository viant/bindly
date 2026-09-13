package locator

// ValueWithMetadata is a reserved provider result envelope. Bindly exposes Value
// through its ordinary APIs and transports Metadata only to an opt-in observer.
// Metadata is opaque, invocation-scoped, and must be immutable or otherwise safe
// for all consumers of a cached resolution. It is never serialized by Bindly.
type ValueWithMetadata struct {
	Value    any
	Metadata any
}

// MetadataScope is an optional capability; existing Scope implementations need
// not implement it. Providers may avoid producing metadata unless requested.
type MetadataScope interface{ MetadataRequested() bool }
