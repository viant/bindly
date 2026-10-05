# Bindly Architecture for Datly 1.0

This document records the final Bindly boundary used by Datly 1.0. Bindly owns
dependency resolution and typed assignment. Datly owns component metadata,
handler execution, SQL reading, Velty execution, and lifecycle orchestration.

## Stable Decisions

### Plans are static

`CompilePlan` produces reusable metadata for one Go target type. A plan stores
field indexes, binding metadata, priority, and optional compiler-supplied
extension data. It never stores a request provider, request value, locator, or
session.

Provider instances do not mutate compiled bindings. Kind-specific metadata must
be produced by the component compiler and placed in `BindingSpec`; runtime
provider hooks are not a second transcription system.

### Scopes are dynamic

`ForScope` creates a child provider registry. A provider registered in the child
shadows the same kind in its parent. Lookup is always performed at bind time,
which allows one component plan to serve many requests.

The child shares immutable plan/type/transformer metadata with its parent but
owns a fresh logical value cache. This prevents request values from leaking
between sibling scopes.

### Providers own data points

Each binding kind is resolved by a registered provider. Bindly has no central
switch over query, path, body, component, state, validator, sequencer, or other
capabilities. Packages may group related provider implementations, but each
kind remains independently registerable and shadowable.

The canonical HTTP kinds are `http_request`, `query`, `path`, `header`,
`cookie`, `form`, and `body`. Multipart form values and file headers can be
resolved through `form`; typed body binding remains independently available.

### Binding metadata is compact

`BindingSpec` carries the compact Datly parameter tag surface plus a full Go
struct tag and optional compiler extension. Rich component schemas, predicates,
codecs, nested object definitions, and DQL ASTs remain in xdatly/Datly metadata;
they are compiled into Bindly plans rather than copied into a Bindly god shape.

The supported `bind` metadata is:

- name, kind, and in
- scope and when
- required and cacheable
- error code and safe error message
- data type and cardinality
- with, async, URI, resource reference, and default value

Tag parsing uses `tagly`. Unknown bind keys fail compilation. There are no
regular-expression parsers, inferred transport fallbacks, or compatibility
parsers for the old `parameter` tag.

### Cache follows Datly parameter semantics

Values are cached by logical parameter name so a resolved component input can
shadow the same logical state reference later in the invocation. The explicit
binding value wins over `locator.CachePolicy`; a provider with no policy is
non-cacheable. Each scope has independent storage and per-name single-flight
locking.

This cache is only for dependency resolution. It is not SQL row caching,
response caching, or persistent storage.

### Resources use io/fs

`resource.Store` registers standard `fs.FS` values under namespaces. `embed.FS`
works without a Bindly-specific embed interface. References use the explicit
`namespace:path` form, or an explicitly registered empty default namespace.
Named namespaces never become implicit defaults. The injector and
transformers can read from the store, and `ResourceFS` returns the unchanged
filesystem for SQL URI or StructQL integration. Resource interpretation remains
with the component compiler or transformer that owns that format.

## Datly Integration Contract

At component registration time Datly resolves package/DQL precedence, target Go
types, tags, and component metadata into input and output `BindingSpec` lists.
It compiles each list once with the application injector.

At invocation time Datly:

1. Creates providers for request data and invocation capabilities.
2. Creates one Bindly child scope.
3. Binds the component input with its compiled plan.
4. Executes the selected reader, custom, or Velty handler.
5. Binds output/state through the same scope where required.
6. Runs the unified output/error lifecycle once.
7. Closes request resources such as multipart temporary files.

Validator, logger, message bus, sequencer, transaction, registry, and similar
capabilities are explicit provider kinds. They are injected only into handlers
that declare them and are not exposed through a generic session data bag.

## Datly Integration Status

Datly compiles component metadata into reusable Bindly plans, preserves the
original `*http.Request`, creates one invocation scope, and uses the request
providers for HTTP data points. Its former parallel request binder and request
provider packages are removed. Reader, custom Go, and Velty handlers all run
through the same scoped engine.

Handler selection remains a component-registry metadata decision; it is not a
dependency lookup. Handler dependencies and capabilities resolve from the one
invocation injector. Handler-owned typed output construction remains valid,
with shared output/error finalization performed by the engine.

SQL URI expansion accepts the same `resource.Store` as `fs.FS`, and the runtime
can receive that exact store with `WithResources`. Declared resources are
resolved during transcription/artifact compilation and fail closed when
missing; runtime parsing and fallback resource maps are forbidden.
