package bindly

import (
	"github.com/viant/bindly/locator"
	"github.com/viant/structology"
	"sync"
	"testing"
)

type registrationOnlyProvider string

func (p registrationOnlyProvider) Kind() string { return string(p) }
func (registrationOnlyProvider) Priority() int  { panic("HasProvider executed Priority") }
func (registrationOnlyProvider) Locate(*structology.State) locator.Locator {
	panic("HasProvider executed Locate")
}

func TestInjectorHasProviderAncestryWithoutResolution(t *testing.T) {
	var absent *Injector
	if absent.HasProvider("generator") {
		t.Fatal("nil injector has a provider")
	}
	empty, err := NewInjector()
	if err != nil {
		t.Fatal(err)
	}
	if empty.HasProvider("generator") || empty.HasProvider("") {
		t.Fatal("empty injector has a provider")
	}
	registry := locator.NewRegistry()
	if err := registry.Register(registrationOnlyProvider("generator")); err != nil {
		t.Fatal(err)
	}
	root, err := NewInjector(WithLocators(registry.Child()), WithProviders(registrationOnlyProvider("root")))
	if err != nil {
		t.Fatal(err)
	}
	child, err := root.ForScope(registrationOnlyProvider("child"))
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := child.ForScope(registrationOnlyProvider("root"))
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"generator", "root", "child"} {
		if !leaf.HasProvider(kind) {
			t.Fatalf("missing ancestor kind %s", kind)
		}
	}
	for _, kind := range []string{"unknown", "Generator", " generator", "generator ", ""} {
		if leaf.HasProvider(kind) {
			t.Fatalf("invented kind %q", kind)
		}
	}
	if root.HasProvider("child") {
		t.Fatal("descendant visible in parent")
	}
	var wg sync.WaitGroup
	for n := 0; n < 32; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				if !leaf.HasProvider("generator") || leaf.HasProvider("unknown") {
					t.Error("unstable lookup")
				}
			}
		}()
	}
	wg.Wait()
}
