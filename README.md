# Bindly

Bindly is a provider-driven dependency injector for Go structs. It compiles
binding metadata once, then resolves values from an invocation scope without
capturing request data in the compiled plan.

## Core Model

- An application `Injector` owns provider definitions, transformers, type
  metadata, and generic resources.
- `CompilePlan` converts component metadata into an immutable reusable plan.
- `ForScope` creates an invocation injector. Local providers shadow parent
  providers by kind and the scope owns its value cache.
- `Bind` resolves each binding from the active scope, transforms and converts
  the value, and writes it to the typed target.

```go
root, err := bindly.NewInjector(bindly.WithProviders(appProviders...))
if err != nil {
    return err
}

plan, err := root.CompilePlan(reflect.TypeOf(Input{}),
    bindly.BindingSpec{
        Path:     "AccountID",
        Name:     "accountID",
        Location: state.Location{Kind: request.QueryKind, In: "accountId"},
    },
)
if err != nil {
    return err
}

requestScope, err := request.New(req,
    request.WithPathParams(pathParams),
)
if err != nil {
    return err
}
defer requestScope.Close()

scope, err := root.ForScope(requestScope.Providers()...)
if err != nil {
    return err
}

input := &Input{}
if err := scope.Bind(ctx, input, bindly.WithPlan(plan)); err != nil {
    return err
}
```

Plans contain field selectors and metadata only. Provider lookup always occurs
against the injector performing `Bind`, so one plan can safely serve sibling
request scopes.

## Bind Tags

For package-defined Go shapes, Bindly can compile tags directly:

```go
type Input struct {
    AccountID int           `bind:"accountID,kind=query,in=accountId,required"`
    IDs       []int         `bind:"ids,kind=query,in=id"`
    Request   *http.Request `bind:"request,kind=http_request"`
}
```

The `bind` tag carries the original Datly parameter-tag surface: positional or
explicit name, `kind`, `in`, `when`, `scope`, `errorCode`, `errorMessage`,
`dataType`, `cardinality`, `with`, `required`, `cacheable`, `async`, `uri`,
`resource`, and `value`. The complete `reflect.StructTag` remains available as
binding metadata, so independent tags such as codec, predicate, description,
and format are not flattened into Bindly-specific fields.

Datly-generated or DQL-defined components should use `BindingSpec` and
`CompilePlan`; they should not synthesize tags and parse them again.

## Request Providers

`provider/request` groups HTTP-related providers as one package while keeping
their data points independent:

- `http_request`
- `query`
- `path`
- `header`
- `cookie`
- `form`
- `body`

Query, header, form, and multipart providers use the destination type: scalar
destinations receive the first repeated value, while slice and array
destinations receive every value. Untyped provider lookup also preserves every
repeated value. Form and body can both resolve typed multipart values and file
headers, matching Datly parameter semantics. The body provider also supports
JSON, named JSON fields, raw bytes or strings, and custom media-type decoders.
Custom decoders can be registered with `request.WithDecoder`. Configure the
multipart memory threshold with `request.WithMaxMultipartMemory`.
`Scope.Close` removes temporary files owned by the cloned multipart request;
header overlays share that cleanup ownership.

A single empty query value is present by default. Opt into treating it as absent
with `request.WithIgnoreEmptyQueryParameters(true)` or the invocation-local
`request.WithQueryPolicy`; repeated values retain their presence.

## Cache Semantics

Each `Bind` owns an invocation cache keyed by provider owner, location and target
type. A binding-level `cacheable` value overrides the provider's
`locator.CachePolicy`; providers without a policy default to non-cacheable.

The injector also owns a value cache for cacheable, source-independent bindings
using an explicit plan. `ForScope` creates a separate value cache. Default
persistent reuse is disabled when binding from a source object or observing
binding metadata. `WithValueCache` explicitly supplies a shared cache;
nonempty `WithSource` objects qualify entries by source identity. Cacheable
resolutions use per-key locking and recheck the cache after acquiring the lock.
Child providers are checked before cached parent values.

Explicit persistent caches store values, not binding metadata. Applications
must manage their lifetime and invalidate them when the underlying provider
changes. `ValueCache.Save` and `Load` support filesystem snapshots; this is not
an HTTP response cache. Raw replay capture bypasses decoded persistent values.

## Binding Options and Compatibility

`WithPlan` selects a compiled contract, and `WithSource` supplies state for
state-dependent providers. `OnlyKinds` and `SkipKinds` restrict the bindings
resolved by a call. Required bindings reject missing or null values by default;
`AllowMissingRequired` explicitly relaxes that policy.

The existing `WithState[T]` / `Inject` entry point uses the same compiled-plan
execution path. Its `WithCache`, `WithAllowedKinds`, `WithDelayedLocator` and
`WithMissingPolicy` options remain available.

`locator.ComposeProviders` resolves named layers in declared order. A found
value, including null, an error, or an authoritative missing value stops
fallback. Layer names must be nonempty and unique; every layer must supply the
same kind. The composed priority is the maximum layer priority, and default
caching requires every layer to permit it.

## Projection, Metadata and Replay

`Plan.Projection` exposes selected bindings under their canonical names or
explicit aliases without creating a second binding pipeline. `WithBindingObserver`
receives binding events, including metadata supplied by metadata-aware locators.

`Plan.Replay` selects bindings for a replay contract. `Capture` snapshots typed
values, `DecodeJSON` accepts replay JSON, and `CaptureSources` snapshots selected
provider sources before transformation, defaults or dependencies run. Providers
implementing `locator.SourceCapturer` can retain raw JSON presence and numbers.
`ReplayPlan.Prepare` declares fresh prerequisite bindings; `WithReplay` applies
the replay through the normal binding and authorization path. See
[replay tests](replay_test.go) and [source capture tests](replay_capture_test.go)
for complete examples.

## Embedded Resources

Bindly accepts any standard `fs.FS`, including `go:embed`:

```go
//go:embed sql/*.sql
var assets embed.FS

injector, err := bindly.NewInjector(
    bindly.WithResourceFS("component", assets),
)
sqlText, err := injector.ReadResource("component:sql/find.sql")
embeddedFS, ok := injector.ResourceFS("component")
```

`ResourceFS` returns the original `fs.FS`, so an `embed.FS` remains available
to SQL URI, StructQL, or other resource-aware compilers. Transformers receive
the same generic resource store. Bindly does not infer
resource type from paths or parse resources with regular-expression heuristics.

`injector.Resources()` exposes that same store for registration-time compilers.
Passing it as an `fs.FS` lets SQL URI and StructQL owners read package assets
without copying them into a parallel map or resource cache. Only an explicitly
registered empty namespace is a default; named namespaces never become an
order-dependent fallback for unqualified references.

## Concurrency

Compiled plans, injector metadata caches, registries, resource stores, and value
caches are safe for concurrent reads and binding after bootstrap. Invocation
scopes should not be shared across unrelated requests. Provider implementations
remain responsible for the concurrency safety of the data they expose.

## Verification

The module requires Go 1.25 or newer. Run against this module's dependencies:

```bash
GOWORK=off GO111MODULE=on go test -mod=readonly ./...
GOWORK=off GO111MODULE=on go test -mod=readonly -race ./...
GOWORK=off GO111MODULE=on go vet -mod=readonly ./...
```
