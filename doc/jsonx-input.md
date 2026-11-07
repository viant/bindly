# Native JSON input options

A native JSON body field can explicitly accept booleans as integer storage values:

```go
Flag *int `json:"flag" jsonx:"boolToInt,null=omit"`
```

`boolToInt` maps JSON true to 1 and false to 0. Existing integer input remains valid,
including values other than 0 and 1. The ordinary typed decoder still rejects
strings, fractions, containers and overflow. No coercion is enabled on untagged fields.

`null=omit` removes that field occurrence from the effective input used for both
value decoding and generated presence markers. Omitted/null-only input leaves the
pointer nil and its marker false. Other fields retain supplied-null presence.
With duplicate keys, omitted null occurrences do not erase an earlier value;
ordinary supplied values retain the decoder's lexical overwrite behavior.

`boolToInt` applies to `int` fields and pointers to `int` fields. `null=omit`
can be used independently on native-supported fields of any type, including
`float64` and `*float64`. Both use the existing canonical public-field plan. Internal members remain excluded before
conversion. Nested rows and named body parameters use the same native provider.
Raw source capture is unchanged. Configured JSON decoders retain representation
ownership; their filtering path does not apply these native conversions. Types
with custom JSON/Text decoding cannot acquire these options over that ownership.
Existing `jsonx:"inline"` handling is unchanged.

This is input-only metadata. Before adopting it in a schema-exposed consumer,
verify its generated HTTP/MCP input schema describes the accepted representations,
its output schema remains unchanged, and the consumer's presence/validation tests
pass. This Bindly implementation alone does not supply consumer schema metadata.
