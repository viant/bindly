package bindly

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/viant/bindly/locator"
	"github.com/viant/bindly/state"
	"github.com/viant/bindly/xform"
	"github.com/viant/structology"
)

type metadataProvider struct {
	value     any
	found     bool
	err       error
	calls     int
	requested []bool
	sources   []any
	resolve   func(context.Context, locator.Scope, string) (any, bool, error)
}

func (p *metadataProvider) Kind() string           { return "metadata" }
func (p *metadataProvider) Priority() int          { return 0 }
func (p *metadataProvider) DefaultCacheable() bool { return true }
func (p *metadataProvider) Locate(source *structology.State) locator.Locator {
	if source != nil {
		p.sources = append(p.sources, source.StatePtr())
	}
	return &metadataLocator{provider: p}
}

type metadataLocator struct{ provider *metadataProvider }

func (l *metadataLocator) Kind() string { return "metadata" }
func (l *metadataLocator) Value(context.Context, reflect.Type, string) (any, bool, error) {
	return nil, false, errors.New("scope required")
}
func (l *metadataLocator) ValueInScope(ctx context.Context, scope locator.Scope, _ reflect.Type, name string) (any, bool, error) {
	p := l.provider
	p.calls++
	requested, ok := scope.(locator.MetadataScope)
	p.requested = append(p.requested, ok && requested.MetadataRequested())
	if p.resolve != nil {
		return p.resolve(ctx, scope, name)
	}
	return p.value, p.found, p.err
}

type metadataIdentityTransform struct{}

func (metadataIdentityTransform) Transform(_ context.Context, _ locator.Resolver, value any) (any, error) {
	return value, nil
}

func TestBindingMetadataConversionPolicy(t *testing.T) {
	type namedInt int
	type namedRecord struct{ ID int }
	for _, tc := range []struct {
		name          string
		value, target any
		source        reflect.Type
		transform     xform.Transformer
		defaultValue  any
		missing, keep bool
	}{
		{"identity", 7, &struct{ Value int }{}, nil, nil, nil, false, true},
		{"assignable named record", struct{ ID int }{7}, &struct{ Value namedRecord }{}, nil, nil, nil, false, true},
		{"assignable interface", []int{7}, &struct{ Value any }{}, nil, nil, nil, false, true},
		{"typed nil", []int(nil), &struct{ Value []int }{}, nil, nil, nil, false, true},
		{"untyped nil", nil, &struct{ Value *int }{}, nil, nil, nil, false, false},
		{"numeric adaptation", 7, &struct{ Value int64 }{}, nil, nil, nil, false, false},
		{"named adaptation", 7, &struct{ Value namedInt }{}, nil, nil, nil, false, false},
		{"source adaptation", 7, &struct{ Value string }{}, reflect.TypeOf(""), nil, nil, false, false},
		{"opaque identity transform", 7, &struct{ Value int }{}, nil, metadataIdentityTransform{}, nil, false, false},
		{"default", nil, &struct{ Value int }{}, nil, nil, 9, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			metadata := &struct{ Label string }{"native evidence"}
			provider := &metadataProvider{value: locator.ValueWithMetadata{Value: tc.value, Metadata: metadata}, found: !tc.missing}
			injector, err := NewInjector(WithProviders(provider))
			if err != nil {
				t.Fatal(err)
			}
			plan, err := injector.CompilePlan(reflect.TypeOf(tc.target), BindingSpec{Path: "Value", Location: state.Location{Kind: "metadata", In: "data"}, SourceType: tc.source, Transformer: tc.transform, DefaultValue: tc.defaultValue})
			if err != nil {
				t.Fatal(err)
			}
			var events []BindingEvent
			err = injector.Bind(context.Background(), tc.target, WithPlan(plan), WithBindingObserver(func(_ context.Context, event BindingEvent) error { events = append(events, event); return nil }))
			if err != nil {
				t.Fatal(err)
			}
			if len(events) != 1 || events[0].Target != tc.target || events[0].Path != "Value" || events[0].Location.In != "data" || !reflect.DeepEqual(events[0].Value, reflect.ValueOf(tc.target).Elem().FieldByName("Value").Interface()) {
				t.Fatalf("events=%+v target=%+v", events, tc.target)
			}
			if (events[0].Metadata == metadata) != tc.keep || len(provider.requested) != 1 || !provider.requested[0] {
				t.Fatalf("metadata=%v requested=%v", events[0].Metadata, provider.requested)
			}
		})
	}
}

func TestBindingMetadataOptInAndInvocationCache(t *testing.T) {
	for _, observe := range []bool{false, true} {
		t.Run(map[bool]string{false: "ordinary", true: "observed"}[observe], func(t *testing.T) {
			metadata := &struct{}{}
			provider := &metadataProvider{value: &locator.ValueWithMetadata{Value: 7, Metadata: metadata}, found: true}
			injector, err := NewInjector(WithProviders(provider))
			if err != nil {
				t.Fatal(err)
			}
			target := struct{ A, B int }{}
			plan, err := injector.CompilePlan(reflect.TypeOf(target), BindingSpec{Path: "A", Location: state.Location{Kind: "metadata"}}, BindingSpec{Path: "B", Location: state.Location{Kind: "metadata"}})
			if err != nil {
				t.Fatal(err)
			}
			options := []BindOption{WithPlan(plan)}
			var events []BindingEvent
			if observe {
				options = append(options, WithBindingObserver(func(_ context.Context, event BindingEvent) error { events = append(events, event); return nil }))
			}
			if err = injector.Bind(context.Background(), &target, options...); err != nil {
				t.Fatal(err)
			}
			if target.A != 7 || target.B != 7 || provider.calls != 1 || provider.requested[0] != observe {
				t.Fatalf("target=%+v calls=%d requested=%v", target, provider.calls, provider.requested)
			}
			if observe && (len(events) != 2 || events[0].Metadata != metadata || events[1].Metadata != metadata) {
				t.Fatalf("cached events=%+v", events)
			}
		})
	}
}

func TestBindingMetadataNestedScopeTargetsAndPublicValue(t *testing.T) {
	type nested struct {
		Value int `bind:"kind=metadata,in=inner"`
	}
	type root struct {
		Value int `bind:"kind=metadata,in=outer"`
	}
	var dependency nested
	provider := &metadataProvider{}
	provider.resolve = func(ctx context.Context, scope locator.Scope, name string) (any, bool, error) {
		if name == "outer" {
			if err := scope.BindTarget(ctx, &dependency); err != nil {
				return nil, false, err
			}
			value, found, err := scope.Value(ctx, &state.Location{Kind: "metadata", In: "inner"})
			if err != nil || !found || value != 7 {
				return nil, false, errors.New("Scope.Value leaked an envelope")
			}
		}
		return locator.ValueWithMetadata{Value: 7, Metadata: name}, true, nil
	}
	injector, err := NewInjector(WithProviders(provider))
	if err != nil {
		t.Fatal(err)
	}
	var input root
	var events []BindingEvent
	if err = injector.Bind(context.Background(), &input, WithBindingObserver(func(_ context.Context, event BindingEvent) error { events = append(events, event); return nil })); err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0].Target != &dependency || events[1].Target != &input || events[0].Metadata != "inner" || events[1].Metadata != "outer" {
		t.Fatalf("events=%+v", events)
	}
	for _, requested := range provider.requested {
		if !requested {
			t.Fatal("nested scope lost metadata request")
		}
	}
	for _, source := range provider.sources {
		if source != &input {
			t.Fatalf("nested binding replaced canonical source: %T", source)
		}
	}
}

func TestBindingObserverErrorFollowsSuccessfulFieldAndMarker(t *testing.T) {
	provider := &metadataProvider{value: locator.ValueWithMetadata{Value: 7, Metadata: "evidence"}, found: true}
	injector, err := NewInjector(WithProviders(provider))
	if err != nil {
		t.Fatal(err)
	}
	type flags struct{ Value bool }
	target := struct {
		Nested struct{ Value int }
		Has    flags
		Later  int
	}{}
	plan, err := injector.CompilePlan(reflect.TypeOf(target), BindingSpec{Path: "Nested.Value", MarkerField: "Has.Value", Location: state.Location{Kind: "metadata"}, ErrorCode: 409}, BindingSpec{Path: "Later", Location: state.Location{Kind: "metadata"}})
	if err != nil {
		t.Fatal(err)
	}
	cause := errors.New("observer rejected evidence")
	err = injector.Bind(context.Background(), &target, WithPlan(plan), WithBindingObserver(func(_ context.Context, event BindingEvent) error {
		if event.Path != "Nested.Value" || event.Target != &target || target.Nested.Value != 7 || !target.Has.Value {
			t.Errorf("observation ran before assignment: %+v %+v", event, target)
		}
		return cause
	}))
	var bindingError *BindingError
	if !errors.Is(err, cause) || !errors.As(err, &bindingError) || bindingError.Path != "Nested.Value" || bindingError.Code != 409 || target.Later != 0 || target.Nested.Value != 7 {
		t.Fatalf("error=%v target=%+v", err, target)
	}
}

func TestBindingMetadataPersistentCacheIsValueOnly(t *testing.T) {
	metadata := make(chan struct{}) // Deliberately not gob-serializable.
	provider := &metadataProvider{value: locator.ValueWithMetadata{Value: 7, Metadata: metadata}, found: true}
	injector, err := NewInjector(WithProviders(provider))
	if err != nil {
		t.Fatal(err)
	}
	type input struct {
		A, B int `bind:"kind=metadata"`
	}
	cache := NewValueCache()
	for pass := 0; pass < 2; pass++ {
		var target input
		var events []BindingEvent
		err = injector.Bind(context.Background(), &target, func(options *bindOptions) { options.cache = cache }, WithBindingObserver(func(_ context.Context, event BindingEvent) error { events = append(events, event); return nil }))
		if err != nil {
			t.Fatal(err)
		}
		if len(events) != 2 || provider.calls != 1 || target.A != 7 || target.B != 7 {
			t.Fatalf("events=%+v calls=%d target=%+v", events, provider.calls, target)
		}
		for _, event := range events {
			if (event.Metadata == metadata) != (pass == 0) {
				t.Fatalf("pass%d metadata=%v", pass, event.Metadata)
			}
		}
	}
	if err = cache.Save(context.Background(), filepath.Join(t.TempDir(), "values.gob")); err != nil {
		t.Fatalf("opaque metadata entered persistent cache: %v", err)
	}
}

func TestBindingMetadataInvalidEnvelopesAndProviderFailure(t *testing.T) {
	cause := errors.New("provider failure")
	for _, tc := range []struct {
		name    string
		value   any
		err     error
		message string
	}{
		{"nil envelope", (*locator.ValueWithMetadata)(nil), nil, "nil metadata envelope"},
		{"nested envelope", locator.ValueWithMetadata{Value: locator.ValueWithMetadata{Value: 7}}, nil, "nested metadata envelopes"},
		{"provider failure", locator.ValueWithMetadata{Value: 7, Metadata: "untrusted"}, cause, "provider failure"},
		{"invalid envelope with provider failure", (*locator.ValueWithMetadata)(nil), cause, "nil metadata envelope"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			provider := &metadataProvider{value: tc.value, found: true, err: tc.err}
			injector, err := NewInjector(WithProviders(provider))
			if err != nil {
				t.Fatal(err)
			}
			target := struct {
				Value int `bind:"kind=metadata"`
			}{}
			observed := false
			err = injector.Bind(context.Background(), &target, WithBindingObserver(func(context.Context, BindingEvent) error { observed = true; return nil }))
			if err == nil || !strings.Contains(err.Error(), tc.message) || observed || target.Value != 0 {
				t.Fatalf("err=%v observed=%v target=%+v", err, observed, target)
			}
			if tc.err != nil && !errors.Is(err, tc.err) {
				t.Fatalf("cause lost: %v", err)
			}
		})
	}
}

func TestMetadataEnvelopeCannotSatisfyRequiredWithNilValue(t *testing.T) {
	provider := &metadataProvider{value: locator.ValueWithMetadata{Value: (*int)(nil), Metadata: "present envelope"}, found: true}
	injector, err := NewInjector(WithProviders(provider))
	if err != nil {
		t.Fatal(err)
	}
	target := struct{ Value *int }{}
	required := true
	plan, err := injector.CompilePlan(reflect.TypeOf(target), BindingSpec{Path: "Value", Location: state.Location{Kind: "metadata"}, Required: &required})
	if err != nil {
		t.Fatal(err)
	}
	observed := false
	err = injector.Bind(context.Background(), &target, WithPlan(plan), WithBindingObserver(func(context.Context, BindingEvent) error { observed = true; return nil }))
	if err == nil || !strings.Contains(err.Error(), "missing required") || observed {
		t.Fatalf("error=%v observed=%v", err, observed)
	}
}
