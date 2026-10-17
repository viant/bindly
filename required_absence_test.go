package bindly_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/viant/bindly"
	"github.com/viant/bindly/input"
	"github.com/viant/bindly/locator"
	"github.com/viant/bindly/provider/request"
	"github.com/viant/bindly/provider/values"
	"github.com/viant/bindly/state"
)

type absenceTarget struct{ Value string }

func bindAbsence(t *testing.T, b bindly.BindingSpec, providers ...locator.Provider) (absenceTarget, error) {
	t.Helper()
	injector, err := bindly.NewInjector(bindly.WithProviders(providers...))
	if err != nil {
		t.Fatal(err)
	}
	plan, err := injector.CompilePlan(reflect.TypeFor[absenceTarget](), b)
	if err != nil {
		t.Fatal(err)
	}
	target := absenceTarget{}
	err = injector.Bind(context.Background(), &target, bindly.WithPlan(plan))
	return target, err
}
func requiredBinding(kind string) bindly.BindingSpec {
	required := true
	return bindly.BindingSpec{Path: "Value", Name: "Jwt", Location: state.Location{Kind: kind, In: "Authorization"}, Required: &required}
}
func TestRequiredAbsenceExternalKinds(t *testing.T) {
	for _, kind := range []string{"query", "path", "header", "cookie", "form", "body"} {
		t.Run(kind, func(t *testing.T) {
			b := requiredBinding(kind)
			_, err := bindAbsence(t, b, values.New(kind, map[string]any{}))
			var failure *bindly.BindingError
			var private *input.Error
			if !errors.As(err, &failure) || !errors.As(err, &private) || err.Error() != "parameter Jwt is required" || failure.StatusCode() != 400 {
				t.Fatalf("required absence = %v", err)
			}
			if !strings.Contains(private.Cause.Error(), "missing required "+kind+" value") {
				t.Fatal("private diagnostic lost")
			}
			if errors.Unwrap(private.Cause) == nil {
				t.Fatal("typed absence lost underlying diagnostic")
			}
		})
	}
	for _, kind := range []string{"component", "generator", "data_view"} {
		b := requiredBinding(kind)
		_, err := bindAbsence(t, b, values.New(kind, map[string]any{}))
		if err == nil || strings.Contains(err.Error(), "parameter Jwt is required") {
			t.Fatalf("internal absence changed: %v", err)
		}
	}
}
func TestRequiredAbsenceNamesAndAuthoredMessage(t *testing.T) {
	for _, tc := range []struct{ name, message, want string }{{"Jwt", "", "parameter Jwt is required"}, {"", "", "parameter Value is required"}, {"Jwt", "authored missing", "authored missing"}, {"Jwt", "invalid: ${error}", `invalid: missing required header value "Authorization"`}} {
		b := requiredBinding("header")
		b.Name = tc.name
		b.ErrorMessage = tc.message
		b.ErrorCode = 401
		_, err := bindAbsence(t, b, values.New("header", map[string]any{}))
		var failure *bindly.BindingError
		if !errors.As(err, &failure) || failure.StatusCode() != 401 || err.Error() != tc.want {
			t.Fatalf("message/status = %v; want %q", err, tc.want)
		}
	}
}

type verifyTransformer struct {
	calls   *int
	failure error
}

func (v verifyTransformer) Transform(_ context.Context, _ locator.Resolver, value interface{}) (interface{}, error) {
	*v.calls++
	return value, v.failure
}
func TestRequiredAbsenceRequestVersusVerification(t *testing.T) {
	for _, tc := range []struct {
		name, token string
		present     bool
		private     string
	}{{name: "missing"}, {name: "empty", present: true}, {name: "malformed", present: true, token: "PRIVATE malformed", private: "PRIVATE malformed token"}, {name: "expired", present: true, token: "PRIVATE expired", private: "PRIVATE expired token"}, {name: "signature", present: true, token: "PRIVATE signature", private: "PRIVATE signature"}, {name: "valid", present: true, token: "valid"}} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/", nil)
			if tc.present {
				r.Header.Set("Authorization", tc.token)
			}
			scope, err := request.New(r)
			if err != nil {
				t.Fatal(err)
			}
			defer scope.Close()
			b := requiredBinding("header")
			b.ErrorCode = 401
			calls := 0
			var private error
			if tc.private != "" {
				private = errors.New(tc.private)
			}
			var failure error
			if private != nil {
				failure = &input.Error{Cause: private}
			}
			b.Transformer = verifyTransformer{calls: &calls, failure: failure}
			_, err = bindAbsence(t, b, scope.Providers()...)
			if !tc.present {
				if err == nil || err.Error() != "parameter Jwt is required" || calls != 0 {
					t.Fatalf("absence ran verifier: %v %d", err, calls)
				}
				return
			}
			if calls != 1 {
				t.Fatalf("existing present/empty semantics changed: %v calls%d", err, calls)
			}
			if private != nil {
				var binding *bindly.BindingError
				if !errors.As(err, &binding) || binding.StatusCode() != 401 || err.Error() != http.StatusText(400) || !errors.Is(err, private) {
					t.Fatalf("verification privacy/status/cause changed: %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
		})
	}
}
func TestRequiredAbsenceDefaultNilAndNestedProvenance(t *testing.T) {
	b := requiredBinding("query")
	b.DefaultValue = "default"
	got, err := bindAbsence(t, b, values.New("query", map[string]any{}))
	if err != nil || got.Value != "default" {
		t.Fatalf("default changed: %v", err)
	}
	b.DefaultValue = nil
	var nilString *string
	_, err = bindAbsence(t, b, values.New("query", map[string]any{"Authorization": nilString}))
	if err == nil || err.Error() != "parameter Jwt is required" {
		t.Fatalf("existing nil absence changed: %v", err)
	}
	_, child := bindAbsence(t, requiredBinding("header"), values.New("header", map[string]any{}))
	// Even a child's exact private cause is not an absence event owned by this binding.
	var childBinding *bindly.BindingError
	if !errors.As(child, &childBinding) {
		t.Fatal(child)
	}
	for _, cause := range []error{child, fmtWrapped{child}, childBinding.Unwrap(), &input.Error{Cause: child}} {
		outer := &bindly.BindingError{Path: "Value", Name: "Outer", Kind: "header", In: "Authorization", Cause: cause}
		if outer.Error() != http.StatusText(400) {
			t.Fatalf("nested absence misattributed: %s", outer.Error())
		}
	}
}

type fmtWrapped struct{ error }

func (w fmtWrapped) Unwrap() error { return w.error }

func TestRequiredAbsenceDoesNotReclassifyConversionOrChildFailure(t *testing.T) {
	required := true
	source := request.NewValues(request.WithHeaders(http.Header{"Authorization": []string{"PRIVATE not an integer"}}))
	injector, err := bindly.NewInjector(bindly.WithProviders(source.Providers()...))
	if err != nil {
		t.Fatal(err)
	}
	target := struct{ Value int }{}
	plan, err := injector.CompilePlan(reflect.TypeOf(target), bindly.BindingSpec{Path: "Value", Name: "Jwt", Location: state.Location{Kind: "header", In: "Authorization"}, Required: &required, ErrorCode: 401})
	if err != nil {
		t.Fatal(err)
	}
	err = injector.Bind(context.Background(), &target, bindly.WithPlan(plan))
	var binding *bindly.BindingError
	var private *input.Error
	if !errors.As(err, &binding) || !errors.As(err, &private) || binding.StatusCode() != 401 || err.Error() != "Bad Request" || !strings.Contains(private.Cause.Error(), "PRIVATE") {
		t.Fatalf("required conversion reclassified/exposed: %v", err)
	}
	_, child := bindAbsence(t, requiredBinding("header"), values.New("header", map[string]any{}))
	var childBinding *bindly.BindingError
	if !errors.As(child, &childBinding) {
		t.Fatal(child)
	}
	for _, nested := range []error{child, childBinding.Unwrap()} {
		b := requiredBinding("header")
		b.Name = "Outer"
		calls := 0
		b.Transformer = verifyTransformer{calls: &calls, failure: nested}
		_, err = bindAbsence(t, b, values.New("header", map[string]any{"Authorization": "present"}))
		if err == nil || err.Error() != "Bad Request" || calls != 1 || !errors.Is(err, nested) {
			t.Fatalf("nested child failure reclassified: %v", err)
		}
	}
}

func TestRequiredAbsenceMessageAccessorOwnership(t *testing.T) {
	for _, literal := range []string{"", "authored safe text", "policy ${error}"} {
		b := requiredBinding("header")
		b.ErrorCode = 401
		b.ErrorMessage = literal
		_, err := bindAbsence(t, b, values.New("header", map[string]any{}))
		var owner *bindly.BindingError
		if !errors.As(err, &owner) {
			t.Fatal("owner missing")
		}
		if text, ok := owner.RequiredAbsenceMessage(); !ok || text != "parameter Jwt is required" {
			t.Fatal("genuine absence accessor failed")
		}
		if literal != "" && literal != "policy ${error}" && owner.Error() != literal {
			t.Fatal("authored precedence changed")
		}
		clone := *owner
		for _, candidate := range []*bindly.BindingError{nil, {Cause: (*input.Error)(nil)}, &clone, {Name: "Parent", Kind: "component", Cause: owner}, {Name: "Reused", Cause: owner.Unwrap()}, {Name: "Conversion", Cause: &input.Error{Cause: errors.New("private")}}, {Name: "Verification", Cause: errors.New("private")}} {
			if _, ok := candidate.RequiredAbsenceMessage(); ok {
				t.Fatal("foreign or copied owner admitted")
			}
		}
	}
}
