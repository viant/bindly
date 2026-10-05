package request

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestRequestProviders(t *testing.T) {
	request, err := http.NewRequest("POST", "http://example.com/test?ids=1&ids=2", strings.NewReader(`{"count":3}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Count", "4")
	request.AddCookie(&http.Cookie{Name: "count", Value: "5"})
	scope, err := New(request, WithPathParams(map[string]string{"id": "6"}))
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		kind, name string
		target     reflect.Type
		want       any
	}{
		{QueryKind, "ids", reflect.TypeOf([]int{}), []string{"1", "2"}},
		{PathKind, "id", reflect.TypeOf(0), "6"},
		{HeaderKind, "x-count", reflect.TypeOf(0), "4"},
		{CookieKind, "count", reflect.TypeOf(0), "5"},
		{BodyKind, "count", reflect.TypeOf(0), 3},
		{HTTPRequestKind, "", reflect.TypeOf(request), request},
	} {
		t.Run(tt.kind, func(t *testing.T) {
			for _, provider := range scope.Providers() {
				if provider.Kind() != tt.kind {
					continue
				}
				value, found, err := provider.Locate(nil).Value(context.Background(), tt.target, tt.name)
				if err != nil || !found || !reflect.DeepEqual(value, tt.want) {
					t.Fatalf("value %#v found %v err %v", value, found, err)
				}
				return
			}
			t.Fatal("missing provider")
		})
	}
}

func TestFormProvider(t *testing.T) {
	request, _ := http.NewRequest("POST", "http://example.com", strings.NewReader(url.Values{"count": {"7"}}.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	scope, err := New(request)
	if err != nil {
		t.Fatal(err)
	}
	for _, provider := range scope.Providers() {
		if provider.Kind() != FormKind {
			continue
		}
		value, found, err := provider.Locate(nil).Value(context.Background(), reflect.TypeOf(0), "count")
		if err != nil || !found || value != "7" {
			t.Fatalf("value %v found %v error %v", value, found, err)
		}
	}
}

func TestHeaderOverlayIsDetached(t *testing.T) {
	request, _ := http.NewRequest("GET", "http://example.com?id=7", nil)
	request.Header.Set("Authorization", "original")
	original, err := New(request, WithPathParams(map[string]string{"id": "8"}))
	if err != nil {
		t.Fatal(err)
	}
	overlay := original.WithHeader("Authorization", "scoped")
	for _, tt := range []struct {
		scope  *Scope
		header string
	}{{original, "original"}, {overlay, "scoped"}} {
		for _, provider := range tt.scope.Providers() {
			switch provider.Kind() {
			case HeaderKind:
				value, found, err := provider.Locate(nil).Value(context.Background(), reflect.TypeOf(""), "Authorization")
				if err != nil || !found || value != tt.header {
					t.Fatalf("header %v found %v err %v", value, found, err)
				}
			case HTTPRequestKind:
				value, _, _ := provider.Locate(nil).Value(context.Background(), nil, "")
				if value != request {
					t.Fatal("request identity changed")
				}
			case PathKind:
				value, _, _ := provider.Locate(nil).Value(context.Background(), reflect.TypeOf(0), "id")
				if value != "8" {
					t.Fatal("lost path")
				}
			}
		}
	}
	if request.Header.Get("Authorization") != "original" {
		t.Fatal("request headers mutated")
	}
	var empty *Scope
	if empty.WithHeader("Authorization", "token") == nil {
		t.Fatal("nil overlay failed")
	}
}

func TestCloseRemovesOwnedMultipartFiles(t *testing.T) {
	var encoded bytes.Buffer
	writer := multipart.NewWriter(&encoded)
	part, err := writer.CreateFormFile("upload", "file.bin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = part.Write(bytes.Repeat([]byte{'x'}, 33<<20)); err != nil {
		t.Fatal(err)
	}
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}
	request, _ := http.NewRequest("POST", "http://example.com", bytes.NewReader(encoded.Bytes()))
	request.Header.Set("Content-Type", writer.FormDataContentType())
	scope, err := New(request)
	if err != nil {
		t.Fatal(err)
	}
	defer scope.Close()
	if request.MultipartForm != nil {
		t.Fatal("scope changed request multipart ownership")
	}
	if scope.cleanup == nil {
		t.Fatal("missing owned multipart cleanup")
	}
	file, err := scope.cleanup.form.File["upload"][0].Open()
	if err != nil {
		t.Fatal(err)
	}
	actual, ok := file.(*os.File)
	if !ok {
		t.Fatalf("expected spilled temp file, got %T", file)
	}
	name := actual.Name()
	file.Close()
	if err = scope.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(name); !os.IsNotExist(err) {
		t.Fatalf("temporary upload still exists: %v", err)
	}
	if err = scope.Close(); err != nil {
		t.Fatalf("Close not idempotent: %v", err)
	}
}

func TestValuesScopePublishesOnlyDeclaredSources(t *testing.T) {
	for _, tt := range []struct {
		name  string
		scope *Scope
		kinds []string
	}{
		{"empty", NewValues(), nil},
		{"path", NewValues(WithPathParams(map[string]string{"id": "1"})), []string{PathKind}},
		{"header overlay", NewValues().WithHeader("Authorization", "token"), []string{HeaderKind}},
		{"explicit empty query", NewValues(WithQuery(nil)), []string{QueryKind}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var kinds []string
			for _, provider := range tt.scope.Providers() {
				kinds = append(kinds, provider.Kind())
			}
			if !reflect.DeepEqual(kinds, tt.kinds) {
				t.Fatalf("kinds %v expected %v", kinds, tt.kinds)
			}
		})
	}
}
