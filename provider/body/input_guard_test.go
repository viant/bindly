package body_test

import (
	"github.com/viant/bindly/provider/body"
	"reflect"
	"testing"
)

type guardChild struct {
	Value *int                  `json:"value" format:"inputUnless=../Canonical" jsonx:"boolToInt"`
	Has   *struct{ Value bool } `json:"-" setMarker:"true"`
}
type guardParent struct {
	Canonical *int                            `json:"canonical"`
	Rows      map[string][]*guardChild        `json:"rows"`
	Has       *struct{ Canonical, Rows bool } `json:"-" setMarker:"true"`
}

func TestBodyAncestorGuard(t *testing.T) {
	for _, raw := range []string{`{"canonical":null,"rows":{"a":[{"value":"bad","Has":{"Value":true}}]}}`, `{"rows":{"a":[{"value":{}}]},"canonical":1}`} {
		p, err := body.New([]byte(raw), "application/json", nil)
		if err != nil {
			t.Fatal(err)
		}
		v, _, err := p.Value(t.Context(), reflect.TypeFor[*guardParent](), "")
		if err != nil {
			t.Fatal(err)
		}
		got := v.(*guardParent)
		if !got.Has.Canonical || got.Rows["a"][0].Value != nil || got.Rows["a"][0].Has.Value {
			t.Fatalf("presence: %#v", got)
		}
	}
}

func TestBodyAncestorGuardDomains(t *testing.T) {
	for _, value := range []string{`"bad"`, `false`, `{}`, `[]`, `1.2`, `9223372036854775808`} {
		for _, parent := range []string{`"canonical":null,`, `"canonical":0,`, ``} {
			raw := `{` + parent + `"rows":{"a":[{"value":` + value + `}]}}`
			p, err := body.New([]byte(raw), "application/json", nil)
			if err != nil {
				t.Fatal(err)
			}
			_, _, err = p.Value(t.Context(), reflect.TypeFor[*guardParent](), "")
			// boolToInt intentionally admits booleans when the field is active.
			wantError := parent == "" && value != "false"
			if (err != nil) != wantError {
				t.Fatalf("%s: %v", raw, err)
			}
		}
	}
	for _, raw := range []string{`{"rows":{"a":[{"value":9223372036854775807}]}}`, `{"rows":{"a":[{"value":-9223372036854775808}]}}`} {
		p, _ := body.New([]byte(raw), "application/json", nil)
		v, _, err := p.Value(t.Context(), reflect.TypeFor[*guardParent](), "")
		if err != nil {
			t.Fatal(err)
		}
		if !v.(*guardParent).Rows["a"][0].Has.Value {
			t.Fatal("active value presence missing")
		}
	}
	p, _ := body.New([]byte(`{"canonical":0,"rows":{"a":[{"value":invalid}]}}`), "application/json", nil)
	if _, _, err := p.Value(t.Context(), reflect.TypeFor[*guardParent](), ""); err == nil {
		t.Fatal("malformed ignored JSON accepted")
	}
}

type guardAdmissionParent struct {
	Canonical *int                                    `json:"canonical" format:"inputGroup=parent,inputPriority=0,inputNames=alias,inputNamesKinds=number|null"`
	Winner    *int                                    `json:"winner" format:"inputGroup=parent,inputPriority=1"`
	Rows      []*guardChild                           `json:"rows"`
	Has       *struct{ Canonical, Winner, Rows bool } `json:"-" setMarker:"true"`
}

func TestBodyGuardUsesAdmittedAncestorSelection(t *testing.T) {
	for _, tc := range []struct {
		raw        string
		suppressed bool
	}{
		{`{"alias":null,"rows":[{"value":"bad"}]}`, true},
		{`{"alias":{},"rows":[{"value":"bad"}]}`, false},
		{`{"canonical":1,"winner":2,"rows":[{"value":"bad"}]}`, false},
		{`{"winner":2,"canonical":1,"rows":[{"value":"bad"}]}`, false},
		{`{"Has":{"Canonical":true},"rows":[{"value":"bad"}]}`, false},
	} {
		p, err := body.New([]byte(tc.raw), "application/json", nil)
		if err != nil {
			t.Fatal(err)
		}
		v, _, err := p.Value(t.Context(), reflect.TypeFor[*guardAdmissionParent](), "")
		if tc.suppressed {
			if err != nil {
				t.Fatal(err)
			}
			got := v.(*guardAdmissionParent)
			if !got.Has.Canonical || got.Rows[0].Has.Value {
				t.Fatal("selected alias presence lost")
			}
		} else if err == nil {
			t.Fatalf("unadmitted ancestor suppressed child: %s", tc.raw)
		}
	}
}

func TestBodyGuardPresencePrecedesNullOmission(t *testing.T) {
	type parent struct {
		Canonical *int                            `json:"canonical" jsonx:"null=omit"`
		Rows      []*guardChild                   `json:"rows"`
		Has       *struct{ Canonical, Rows bool } `json:"-" setMarker:"true"`
	}
	p, err := body.New([]byte(`{"canonical":null,"rows":[{"value":"ignored"}]}`), "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	v, _, err := p.Value(t.Context(), reflect.TypeFor[*parent](), "")
	if err != nil {
		t.Fatal(err)
	}
	got := v.(*parent)
	if got.Has.Canonical || got.Rows[0].Has.Value {
		t.Fatal("source selection and converted presence conflated")
	}
}
