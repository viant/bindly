package bindly

import (
	"context"
	"github.com/viant/bindly/state"
	"reflect"
	"sync"
	"testing"
)

type borrowedAuthorization struct{ roles map[string]bool }
type borrowedRecord struct {
	Auth *borrowedAuthorization
	Rows []int
	Has  *borrowedMarkers `setMarker:"true"`
}
type borrowedMarkers struct{ Auth, Rows bool }

func TestResolvedComponentReferenceIdentityAndReadOnlySharing(t *testing.T) {
	inj, _ := NewInjector()
	plan, err := inj.CompilePlan(reflect.TypeFor[borrowedRecord](), BindingSpec{Path: "Auth", Location: state.Location{Kind: "component", In: "GET:/authorization"}}, BindingSpec{Path: "Rows", Location: state.Location{Kind: "query", In: "rows"}})
	if err != nil {
		t.Fatal(err)
	}
	auth := &borrowedAuthorization{roles: map[string]bool{"reader": true}}
	seed := &borrowedRecord{Auth: auth, Rows: []int{7}}
	var wg sync.WaitGroup
	for range 24 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out := &borrowedRecord{}
			err := inj.Bind(context.Background(), out, WithPlan(plan), WithResolvedInput(plan, seed, "Auth", "Rows"))
			if err != nil {
				t.Error(err)
				return
			}
			if out.Auth != auth || !out.Auth.roles["reader"] || out.Has == nil || !out.Has.Auth {
				t.Error("component identity, private role data or canonical marker lost")
			}
			out.Rows[0] = 9
			if seed.Rows[0] != 7 {
				t.Error("ordinary rows alias seed")
			}
		}()
	}
	wg.Wait()
}

func TestResolvedComponentBorrowDoesNotExtendScopedOrNoncacheableData(t *testing.T) {
	for _, spec := range []BindingSpec{
		{Path: "Auth", Location: state.Location{Kind: "query", In: "auth"}},
		{Path: "Auth", Location: state.Location{Kind: "component", In: "GET:/authorization"}, Scope: "request"},
		{Path: "Auth", Location: state.Location{Kind: "component", In: "GET:/authorization"}, Cacheable: func() *bool { v := false; return &v }()},
	} {
		inj, _ := NewInjector()
		plan, err := inj.CompilePlan(reflect.TypeFor[borrowedRecord](), spec)
		if err != nil {
			t.Fatal(err)
		}
		seed := &borrowedRecord{Auth: &borrowedAuthorization{roles: map[string]bool{"reader": true}}}
		if err := inj.Bind(context.Background(), &borrowedRecord{}, WithPlan(plan), WithResolvedInput(plan, seed, "Auth")); err == nil {
			t.Fatal("ineligible binding borrowed private mutable data")
		}
	}
}
