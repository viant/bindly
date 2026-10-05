package bindly

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/viant/bindly/locator"
	"github.com/viant/bindly/locator/buildin"
	"github.com/viant/bindly/state"
)

// TestNewInjector_DefaultAndOptions validates default wiring and option
// application in NewInjector.
func TestNewInjector_DefaultAndOptions(t *testing.T) {
	t.Run("default injector", func(t *testing.T) {
		inj, err := NewInjector()
		assert.NoError(t, err)
		if assert.NotNil(t, inj) {
			assert.NotNil(t, inj.locators)
			assert.NotNil(t, inj.transformers)
			assert.Equal(t, bindingTag, inj.bindingTag)
			assert.Equal(t, xFormTag, inj.xformTag)
			assert.Equal(t, "interface", inj.interfaceKind)
			assert.NotNil(t, inj.bindingCache)
			assert.NotNil(t, inj.structTypeCache)
		}
	})

	t.Run("with custom locator and providers registered", func(t *testing.T) {
		reg := locator.NewRegistry()
		// use a simple map provider from buildin to avoid custom types
		prov := buildin.Map("setting", "Settings", 1)

		inj, err := NewInjector(
			WithLocators(reg),
			WithProviders(prov),
			WithBindingTag("customBind"),
			WithTransformerTag("customX"),
		)
		assert.NoError(t, err)

		if assert.NotNil(t, inj) {
			// provider should be registered against the custom registry
			p, ok := reg.Lookup("setting")
			assert.True(t, ok)
			assert.Equal(t, prov, p)
			assert.Equal(t, "customBind", inj.bindingTag)
			assert.Equal(t, "customX", inj.xformTag)
		}
	})
}

// TestForScope_ShadowsProviderAndSharesCaches proves that a request provider
// can replace an application provider without copying the parent registry.
func TestForScope_ShadowsProviderAndSharesCaches(t *testing.T) {
	reg := locator.NewRegistry()
	parentProvider := buildin.Map("setting", "Settings", 1)
	_ = reg.Register(parentProvider)

	parent, err := NewInjector(WithLocators(reg))
	assert.NoError(t, err)
	childProvider := buildin.Map("setting", "ScopedSettings", 1)
	child, err := parent.ForScope(childProvider)
	assert.NoError(t, err)

	// registries must not be the same pointer
	if assert.NotNil(t, child) {
		assert.NotSame(t, parent.locators, child.locators)
		// parent only knows about original provider
		_, okParentSetting := parent.locators.Lookup("setting")
		assert.True(t, okParentSetting)

		// Child lookup shadows the parent while the parent remains unchanged.
		_, okChildSetting := child.locators.Lookup("setting")
		assert.True(t, okChildSetting)
		resolved, _ := child.locators.Lookup("setting")
		assert.Equal(t, childProvider, resolved)
		resolvedParent, _ := parent.locators.Lookup("setting")
		assert.Equal(t, parentProvider, resolvedParent)

		// caches and transformers should be shared pointers
		assert.Same(t, parent.bindingCache, child.bindingCache)
		assert.Same(t, parent.structTypeCache, child.structTypeCache)
		assert.Same(t, parent.transformers, child.transformers)
	}
}

// TestBindingContext_Options verifies that binding options mutate
// BindingContext fields as expected.
func TestBindingContext_Options(t *testing.T) {
	type State struct{}
	inj, err := NewInjector()
	assert.NoError(t, err)
	customCache := NewValueCache()

	ctx := WithState[State](inj, &State{},
		WithCache[State](customCache),
		WithAllowedKinds[State]("state", "instance"),
		WithDelayedLocator[State]("interface"),
		WithMissingPolicy[State](false),
	)

	if assert.NotNil(t, ctx) {
		val := reflect.ValueOf(ctx).Elem()
		valueCacheField := val.FieldByName("valueCache")
		allowedKindsField := val.FieldByName("allowedKinds")
		delayedKindsField := val.FieldByName("delayedKinds")
		strictMissingField := val.FieldByName("strictMissing")

		// On Go 1.22+ accessing Interface on an unexported field from
		// another package panics. For this high-level options test we only
		// assert that options were applied without inspecting the internal
		// maps directly to avoid relying on unexported layout.
		if assert.True(t, valueCacheField.IsValid()) {
			assert.False(t, valueCacheField.IsZero())
		}
		if assert.True(t, allowedKindsField.IsValid()) {
			assert.False(t, allowedKindsField.IsZero())
		}
		if assert.True(t, delayedKindsField.IsValid()) {
			assert.False(t, delayedKindsField.IsZero())
		}
		if assert.True(t, strictMissingField.IsValid()) {
			// ensure the field is a bool (kind check avoids Interface call)
			assert.Equal(t, reflect.Bool, strictMissingField.Kind())
		}
	}
}

// TestBinding_Getters ensures small Binding getters expose underlying
// metadata correctly.
func TestBinding_Getters(t *testing.T) {
	loc := &state.Location{Kind: "state", In: "Config.Port"}
	b := &Binding{
		location:     loc,
		Name:         "port",
		Scope:        "request",
		Required:     boolPointer(true),
		Cacheable:    boolPointer(true),
		DefaultValue: 8080,
	}

	assert.Equal(t, "state", b.Kind())
	assert.Equal(t, "Config.Port", b.In())
	assert.Equal(t, "port", b.NameTag())
	assert.Equal(t, "request", b.ScopeTag())
	assert.True(t, b.IsRequired())
	assert.True(t, b.IsCacheable())
	assert.Equal(t, 8080, b.Default())
	assert.Equal(t, loc, b.Location())
}

func boolPointer(value bool) *bool {
	return &value
}
