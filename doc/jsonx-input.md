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

Additional opt-in conversions share Tagly's `jsonx.Compile` policy:

| Option | Storage | Additional accepted input |
| --- | --- | --- |
| `intToBool` | bool | Integer tokens 0 and 1 |
| `numberToString` | string | Any JSON number, preserving its exact token text |
| `commaStringToSlice` | []string | Comma-separated strings, trimming each cell and dropping empty cells |
| `boolIntToYN` | string | Booleans, native signed integers, and Y/N/0/1/empty strings |

`intToBool` rejects other integers and numeric strings. `boolIntToYN` maps zero
and false to N, nonzero integers and true to Y; it uppercases strings without
trimming, maps 0/1 to N/Y, and preserves an empty string. Integer conversions
reject fractions, exponent spellings and overflow. `numberToString` instead
accepts arbitrary valid numeric lexemes, including large integers and exponents,
without floating-point conversion. Existing strings remain unchanged.

`commaStringToSlice` preserves existing array cells, including whitespace; it
has no quoted CSV semantics. Empty/all-empty scalar strings become null, while
an empty JSON array remains an empty array. Conversion-produced null is supplied
input and retains presence even when `null=omit` is authored; `null=omit` applies
only to original null occurrences. Other rules also preserve ordinary null
semantics unless `null=omit` is explicitly authored.

Conversions run after alias winner selection. Losing aliases are not converted;
malformed earlier occurrences within the winning representation still fail.
Only one value conversion may be authored per field. Shared `Policy.InputDomain`
metadata describes input branches and exact integer bounds; HTTP/MCP owners must
consume it with native-decoder ownership enabled. These tags do not install
conversion in standard encoding/json or standalone Structology decoding.

## Native unknown-field admission

Use `body.WithUnknownFieldPolicy(structjson.ErrorOnUnknown)` to reject undeclared
members through the native body decoder. This retains declared `jsonx` input
conversion, aliases, internal-field filtering, default dates and presence
reconstruction in their existing order. Untagged fields keep ordinary JSON
types. The default remains `structjson.IgnoreUnknown`.

The policy applies to JSON media types and cannot be combined with a configured
decoder for that media type: a configured decoder owns its own input contract.
Unsupported combinations fail when the source is constructed. Custom
`UnmarshalJSON` and `UnmarshalText` values continue owning their full subtrees.

## Per-binding body representation

`BindingSpec.BodyRepresentation` supplies immutable metadata over the binding's
linked body source type. Native body sources retain their own unknown-field
policy, configured decoder, aliases, bytes and presence handling. Independently
represented named bindings can therefore share one request source without
rebuilding a source or decoding its JSON twice.

Plan compilation rejects incompatible source types, non-body bindings and
presence-only bindings with representation metadata. A source that cannot honor
the native representation capability returns an error. Known-hidden and canonical
internal fields retain their existing filtering behavior; arbitrary unknown
members still follow the original source's unknown-field policy. Existing caches
distinguish canonical, represented and revised-metadata results.
