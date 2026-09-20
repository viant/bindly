package bindly

import (
	"github.com/viant/bindly/state"
	"reflect"
	"testing"
)

func TestPlanSnapshotPresence(t *testing.T) {
	type input struct {
		ID  int
		Has *struct{ ID bool } `setMarker:"true"`
	}
	injector, err := NewInjector()
	if err != nil {
		t.Fatal(err)
	}
	plan, err := injector.CompilePlan(reflect.TypeOf(input{}), BindingSpec{Path: "ID", Location: state.Location{Kind: "query", In: "id"}})
	if err != nil {
		t.Fatal(err)
	}
	value := &input{}
	for _, present := range []bool{false, true, false} {
		if err := plan.SetPresence(value, "ID", present); err != nil {
			t.Fatal(err)
		}
		actual, err := plan.Presence(value, "ID")
		if err != nil || actual != present {
			t.Fatalf("presence=%v: %v", actual, err)
		}
	}
	var absent *input
	if _, err := plan.Presence(absent, "ID"); err == nil {
		t.Fatal("nil target accepted")
	}
	if err := plan.SetPresence(value, "Missing", true); err == nil {
		t.Fatal("unknown binding accepted")
	}
}
