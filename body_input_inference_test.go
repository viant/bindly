package bindly

import (
	"context"
	"github.com/viant/bindly/provider/body"
	"github.com/viant/bindly/state"
	"reflect"
	"testing"
)

// These tags did not opt a body binding into a root conversion before
// objectToSlice was introduced. Preserve their existing typed source behavior.
func TestBodyInputTagInferencePreservesUnrelatedMetadata(t *testing.T) {
	for _, tc := range []struct {
		name, raw string
		typ       reflect.Type
	}{
		{"selected scalar", `{"value":true}`, reflect.TypeOf(struct {
			Value bool `parameter:"kind=body,in=value" jsonx:"intToBool"`
		}{})},
		{"selected null omission", `{"value":null}`, reflect.TypeOf(struct {
			Value *int `parameter:"kind=body,in=value" jsonx:"null=omit"`
		}{})},
		{"whole scalar", `true`, reflect.TypeOf(struct {
			Value bool `parameter:"kind=body" jsonx:"intToBool"`
		}{})},
		{"whole null omission", `null`, reflect.TypeOf(struct {
			Value *int `parameter:"kind=body" jsonx:"null=omit"`
		}{})},
		{"unrelated future metadata", `1`, reflect.TypeOf(struct {
			Value int `parameter:"kind=body" jsonx:"futureOption"`
		}{})},
		{"selected collection", `{"value":[]}`, reflect.TypeOf(struct {
			Value []rootPair `parameter:"kind=body,in=value" jsonx:"objectToSlice"`
		}{})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source, err := body.New([]byte(tc.raw), "application/json", nil)
			if err != nil {
				t.Fatal(err)
			}
			injector, err := NewInjector(WithProviders(source))
			if err != nil {
				t.Fatal(err)
			}
			plan, err := injector.CompilePlan(tc.typ)
			if err != nil {
				t.Fatal(err)
			}
			target := reflect.New(tc.typ).Interface()
			if err := injector.Bind(context.Background(), target, WithPlan(plan)); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestBodyInputTagExplicitPolicyStillValidates(t *testing.T) {
	injector, _ := NewInjector()
	for _, tc := range []struct {
		location state.Location
		tag      string
	}{
		{state.Location{Kind: "body", In: "selected"}, `jsonx:"objectToSlice"`},
		{state.Location{Kind: "body"}, `jsonx:"intToBool"`},
		{state.Location{Kind: "body"}, `jsonx:"null=omit"`},
	} {
		spec := BindingSpec{Path: "Value", Location: tc.location}
		metadata := reflect.ValueOf(&spec).Elem().FieldByName("BodyInputTag")
		if !metadata.IsValid() {
			t.Fatal("explicit body input policy API missing")
		}
		metadata.SetString(tc.tag)
		if _, err := injector.CompilePlan(reflect.TypeOf(struct{ Value []rootPair }{}), spec); err == nil {
			t.Fatalf("accepted explicit policy %s at %+v", tc.tag, tc.location)
		}
	}
}
