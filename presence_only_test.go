package bindly_test

import (
	"context"
	"fmt"
	"github.com/stretchr/testify/require"
	"github.com/viant/bindly"
	"github.com/viant/bindly/locator"
	"github.com/viant/bindly/provider/body"
	"reflect"
	"testing"
)

type presenceInput struct {
	Supplied bool                     `parameter:"Supplied,kind=body,in=channelV2,presenceOnly=true"`
	Has      *struct{ Supplied bool } `json:"-" setMarker:"true"`
}

func TestPresenceOnlyBodyDoesNotDecodeValues(t *testing.T) {
	injector, err := bindly.NewInjector()
	require.NoError(t, err)
	plan, err := injector.CompilePlan(reflect.TypeFor[presenceInput]())
	require.NoError(t, err)
	for _, raw := range []string{`null`, `true`, `false`, `0`, `1e1000`, `-1e1000`, `"bad"`, `{"x":1e1000}`, `[1e1000]`} {
		t.Run(raw, func(t *testing.T) {
			provider, err := body.New([]byte(`{"channelV2":`+raw+`}`), "application/json", nil)
			require.NoError(t, err)
			scope, err := injector.ForScope(provider)
			require.NoError(t, err)
			var actual presenceInput
			require.NoError(t, scope.Bind(t.Context(), &actual, bindly.WithPlan(plan)))
			require.True(t, actual.Supplied)
			require.True(t, actual.Has.Supplied)
		})
	}
	for _, raw := range []string{`{}`, `{"other":1e1000}`} {
		provider, _ := body.New([]byte(raw), "application/json", nil)
		scope, _ := injector.ForScope(provider)
		var actual presenceInput
		require.NoError(t, scope.Bind(t.Context(), &actual, bindly.WithPlan(plan)))
		require.False(t, actual.Supplied)
	}
}

func TestPresenceOnlySyntaxSelectionAndReplay(t *testing.T) {
	injector, err := bindly.NewInjector()
	require.NoError(t, err)
	plan, err := injector.CompilePlan(reflect.TypeFor[presenceInput]())
	require.NoError(t, err)
	for _, tc := range []struct {
		raw           string
		present, fail bool
	}{
		{`{"CHANNELV2":null}`, true, false},
		{`{"channelV2":1e1000,"channelV2":0}`, true, false},
		{`{"channelV2":null,"CHANNELV2":{}}`, true, false},
		{`{"CHANNELV2":1,"ChannelV2":2}`, false, true},
		{`{"channelV2":invalid}`, false, true},
		{`{"channelV2":0,"other":invalid}`, false, true},
		{`[]`, false, true},
		{`null`, false, false},
	} {
		provider, _ := body.New([]byte(tc.raw), "application/json", nil)
		composed, err := locator.ComposeProviders("body", locator.ProviderLayer{Name: "request", Provider: provider})
		require.NoError(t, err)
		scope, err := injector.ForScope(composed)
		require.NoError(t, err)
		var actual presenceInput
		err = scope.Bind(t.Context(), &actual, bindly.WithPlan(plan))
		if tc.fail {
			require.Error(t, err)
			continue
		}
		require.NoError(t, err)
		require.Equal(t, tc.present, actual.Supplied)
		replayPlan, err := plan.Replay("Supplied")
		require.NoError(t, err)
		replay, err := replayPlan.CaptureSources(t.Context(), []locator.Provider{composed})
		require.NoError(t, err)
		var restored presenceInput
		require.NoError(t, injector.Bind(t.Context(), &restored, bindly.WithPlan(plan), bindly.WithReplay(bindly.ReplayBinding{Replay: replay, Only: true})))
		require.Equal(t, actual.Supplied, restored.Supplied)
		providers, err := replay.Providers()
		require.NoError(t, err)
		replayScope, err := injector.ForScope(providers...)
		require.NoError(t, err)
		var projected presenceInput
		require.NoError(t, replayScope.Bind(t.Context(), &projected, bindly.WithPlan(plan)))
		require.Equal(t, actual.Supplied, projected.Supplied)
		_, _, err = providers[0].Locate(nil).Value(t.Context(), reflect.TypeFor[bool](), "channelV2")
		require.ErrorContains(t, err, "no value authority")
	}
}

func TestPresenceOnlyCacheIsSeparateFromValue(t *testing.T) {
	type input struct {
		Supplied bool `parameter:"kind=body,in=value,presenceOnly=true"`
		Value    bool `parameter:"kind=body,in=value"`
	}
	injector, err := bindly.NewInjector()
	require.NoError(t, err)
	plan, err := injector.CompilePlan(reflect.TypeFor[input]())
	require.NoError(t, err)
	p, _ := body.New([]byte(`{"value":false}`), "application/json", nil)
	scope, err := injector.ForScope(p)
	require.NoError(t, err)
	cache := bindly.NewValueCache()
	for i := 0; i < 2; i++ {
		var got input
		require.NoError(t, scope.Bind(t.Context(), &got, bindly.WithPlan(plan), bindly.WithValueCache(cache)))
		require.True(t, got.Supplied)
		require.False(t, got.Value)
	}
}

func TestPresenceOnlyRejectsInvalidConfiguration(t *testing.T) {
	injector, err := bindly.NewInjector()
	require.NoError(t, err)
	for _, tag := range []string{`parameter:"kind=query,in=x,presenceOnly=true"`, `parameter:"kind=body,presenceOnly=true"`, `parameter:"kind=body,in=x,presenceOnly=true,default=true"`, `parameter:"kind=body,in=x,presenceOnly=bad"`} {
		typ := reflect.StructOf([]reflect.StructField{{Name: "Value", Type: reflect.TypeFor[bool](), Tag: reflect.StructTag(tag)}})
		_, err := injector.CompilePlan(typ)
		require.Error(t, err, tag)
	}
}

func TestPresenceOnlyProviderPolicyAndConcurrency(t *testing.T) {
	p, err := body.New([]byte(`{"CHANNELV2":1e1000}`), "application/json", map[string]string{"alias": "channelV2"})
	require.NoError(t, err)
	found, err := p.Presence(t.Context(), "alias")
	require.NoError(t, err)
	require.True(t, found)
	exact, _ := body.New([]byte(`{"CHANNELV2":1e1000}`), "application/json", nil, body.WithExactFieldNames())
	found, err = exact.Presence(t.Context(), "channelV2")
	require.NoError(t, err)
	require.False(t, found)
	for _, media := range []string{"text/plain", "application/x-www-form-urlencoded"} {
		p, _ := body.New([]byte(`{"channelV2":1}`), media, nil)
		_, err := p.Presence(t.Context(), "channelV2")
		require.Error(t, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = p.Presence(ctx, "channelV2")
	require.ErrorIs(t, err, context.Canceled)
	for i := 0; i < 12; i++ {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Parallel()
			found, err := p.Presence(t.Context(), "alias")
			require.NoError(t, err)
			require.True(t, found)
		})
	}
}

func TestPresenceOnlyComposedFallback(t *testing.T) {
	injector, err := bindly.NewInjector()
	require.NoError(t, err)
	plan, err := injector.CompilePlan(reflect.TypeFor[presenceInput]())
	require.NoError(t, err)
	absent, _ := body.New([]byte(`{}`), "application/json", nil)
	present, _ := body.New([]byte(`{"channelV2":1e1000}`), "application/json", nil)
	composed, err := locator.ComposeProviders("body", locator.ProviderLayer{Name: "first", Provider: absent}, locator.ProviderLayer{Name: "fallback", Provider: present})
	require.NoError(t, err)
	scope, err := injector.ForScope(composed)
	require.NoError(t, err)
	var got presenceInput
	require.NoError(t, scope.Bind(t.Context(), &got, bindly.WithPlan(plan)))
	require.True(t, got.Supplied)
}
