# Whole-body record collection input

A whole-body slice can opt into the native Tagly `objectToSlice` input policy:

```go
type Input struct {
    Body []Pair `parameter:"kind=body,in=" jsonx:"objectToSlice"`
}
```

`CompilePlan` retains the authored field policy as `BindingSpec.BodyInputTag`.
A compiler can also supply that `reflect.StructTag` explicitly; conflicting
authored metadata is rejected. `ValidateBodyInputTag` validates the target and
binding source. Only direct slices of records are admitted, including named
slices. Slice or record JSON/Text owners, source type adaptations, transformers,
presence-only binding, and `bodyNullPolicy` conflict with this input contract.
Only whole-body fields declaring `objectToSlice` infer this root policy. Other
field metadata, including selected body fields and unrelated `jsonx` options,
retains its previous behavior. Explicit `BodyInputTag` on a selected body member
is rejected. Ordinary nested record
members continue using the existing native `jsonx` field pipeline.

A supplied object decodes as one record. Arrays keep their order and ordinary
record decoding, including null record elements. Missing body remains absent;
null remains present and nil; `[]` remains present and empty. The optional
`null=omit` flag makes an explicit root null absent. Required binding continues
to reject absent or nil values. Invalid scalar roots and malformed JSON fail.

The body locator's `ValueWithBodyInputTag` consumes the policy once before the
existing public filtering, typed decoding and presence pipeline. The same
converted bytes feed all three stages. Linked element representation metadata,
explicit names, visibility and presence remain authoritative. Registered custom
decoders reject this policy instead of changing their input contract. The native
default decoder registry is supported. Non-JSON media and aliased selected body
members reject the root policy.

Policies are per binding, not mutable provider options. Deferred sources forward
them at lookup time; resolution caches distinguish the exact root tag. Replay
preserves original source JSON and projects it under the receiving binding's
policy, including strict receivers. Output serialization is unchanged. Schema
consumers must use Tagly's Object/Array input domain with the same complete record
schema for the object alternative and array items; this package does not generate
HTTP or MCP schemas.
