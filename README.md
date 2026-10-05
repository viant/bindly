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
`Scope.Close` removes multipart temporary files.

## Cache Semantics

The value cache is invocation-scoped and keyed by logical binding name, matching
Datly parameter shadowing semantics. A binding-level `cacheable` value overrides
the provider's `locator.CachePolicy`; providers without a policy default to
non-cacheable. Cacheable values use per-name single-flight locking and are
rechecked after lock acquisition.

`ValueCache` is not a response cache or persistent storage mechanism.

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

This module targets Go 1.23.1. Run:

```bash
GOTOOLCHAIN=go1.23.1 go test ./...
GOTOOLCHAIN=go1.23.1 go test -race ./...
GOTOOLCHAIN=go1.23.1 go vet ./...
```
