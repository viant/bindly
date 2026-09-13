package request

import (
	"context"
	"net/url"
	"reflect"
	"testing"

	"github.com/viant/bindly"
	"github.com/viant/bindly/provider/values"
)

func TestEmptyQueryPolicy(t *testing.T) {
	for _, tt := range []struct {
		name          string
		query         []string
		ignore, found bool
	}{
		{"present empty default", []string{""}, false, true},
		{"ignored empty", []string{""}, true, false},
		{"ignored zero values", []string{}, true, false},
		{"repeated empty preserved", []string{"", ""}, true, true},
		{"mixed preserved", []string{"", "x"}, true, true},
		{"zero is present", []string{"0"}, true, true},
		{"false is present", []string{"false"}, true, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			scope := NewValues(WithQuery(url.Values{"q": tt.query}), WithIgnoreEmptyQueryParameters(tt.ignore))
			value, found, err := scope.Query().Locate(nil).Value(context.Background(), reflect.TypeOf([]string{}), "q")
			if err != nil || found != tt.found {
				t.Fatalf("value=%v found=%t error=%v", value, found, err)
			}
			if found && !reflect.DeepEqual(value, tt.query) {
				t.Fatalf("query changed: %v", value)
			}
		})
	}
}

func TestIgnoredEmptyQueryFallsBackToParent(t *testing.T) {
	root, err := bindly.NewInjector(bindly.WithProviders(values.New("query", map[string]any{"name": "parent"})))
	if err != nil {
		t.Fatal(err)
	}
	for _, ignore := range []bool{false, true} {
		scope := NewValues(WithQuery(url.Values{"name": {""}}), WithIgnoreEmptyQueryParameters(ignore))
		child, err := root.ForScope(scope.Providers()...)
		if err != nil {
			t.Fatal(err)
		}
		var input struct {
			Name string `bind:"kind=query,in=name,required"`
		}
		if err := child.Bind(context.Background(), &input); err != nil {
			t.Fatal(err)
		}
		want := ""
		if ignore {
			want = "parent"
		}
		if input.Name != want {
			t.Fatalf("ignore=%t got=%q want=%q", ignore, input.Name, want)
		}
	}
}

func TestQueryPolicyContextIsolation(t *testing.T) {
	scope := NewValues(WithQuery(url.Values{"q": {""}}), WithIgnoreEmptyQueryParameters(true))
	provider := scope.Query().Locate(nil)
	for _, ignore := range []bool{false, true} {
		t.Run(map[bool]string{false: "preserve", true: "ignore"}[ignore], func(t *testing.T) {
			t.Parallel()
			ctx := WithQueryPolicy(context.Background(), QueryPolicy{IgnoreEmptyParameters: ignore})
			for i := 0; i < 10; i++ {
				_, found, err := provider.Value(ctx, reflect.TypeOf(""), "q")
				if err != nil || found == ignore {
					t.Fatalf("ignore=%t found=%t error=%v", ignore, found, err)
				}
			}
		})
	}
}
