package request_test

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/viant/bindly"
	inputerror "github.com/viant/bindly/input"
	"github.com/viant/bindly/locator"
	bodyprovider "github.com/viant/bindly/provider/body"
	requestprovider "github.com/viant/bindly/provider/request"
	"github.com/viant/bindly/state"
)

func TestRequestProvidersBindCanonicalDataPoints(t *testing.T) {
	type payloadHas struct {
		Active bool
	}
	type payload struct {
		Active bool        `json:"active"`
		Has    *payloadHas `setMarker:"true"`
	}
	type inputHas struct {
		PathID  bool
		Payload bool
	}
	type input struct {
		Request *http.Request
		PathID  int
		Names   []string
		Token   string
		Tokens  []string
		Session string
		Method  string
		Payload payload
		Has     *inputHas `setMarker:"true"`
	}
	root, err := bindly.NewInjector()
	if err != nil {
		t.Fatalf("NewInjector() error = %v", err)
	}
	plan, err := root.CompilePlan(reflect.TypeOf(input{}),
		bindly.BindingSpec{Path: "Request", Location: state.Location{Kind: requestprovider.RequestKind}},
		bindly.BindingSpec{Path: "PathID", Location: state.Location{Kind: requestprovider.PathKind, In: "id"}},
		bindly.BindingSpec{Path: "Names", Location: state.Location{Kind: requestprovider.QueryKind, In: "name"}},
		bindly.BindingSpec{Path: "Token", Location: state.Location{Kind: requestprovider.HeaderKind, In: "X-Token"}},
		bindly.BindingSpec{Path: "Tokens", Location: state.Location{Kind: requestprovider.HeaderKind, In: "X-Token"}},
		bindly.BindingSpec{Path: "Session", Location: state.Location{Kind: requestprovider.CookieKind, In: "session"}},
		bindly.BindingSpec{Path: "Method", Location: state.Location{Kind: requestprovider.RequestKind, In: "method"}},
		bindly.BindingSpec{Path: "Payload", Location: state.Location{Kind: requestprovider.BodyKind}},
	)
	if err != nil {
		t.Fatalf("CompilePlan() error = %v", err)
	}
	req := httptest.NewRequest("POST", "/users?name=ignored", bytes.NewBufferString(`{"active":true}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "session", Value: "abc123"})
	scope, err := requestprovider.New(req,
		requestprovider.WithPathParams(map[string]string{"id": "42"}),
		requestprovider.WithQuery(url.Values{"name": []string{"Ada", "Grace"}}),
		requestprovider.WithHeaders(http.Header{"X-Token": []string{"secret", "secondary"}}),
	)
	if err != nil {
		t.Fatalf("request.New() error = %v", err)
	}
	injector, err := root.ForScope(scope.Providers()...)
	if err != nil {
		t.Fatalf("ForScope() error = %v", err)
	}
	actual := &input{}
	if err := injector.Bind(context.Background(), actual, bindly.WithPlan(plan)); err != nil {
		t.Fatalf("Bind() error = %v", err)
	}
	if actual.Request != req || actual.PathID != 42 || actual.Token != "secret" || actual.Session != "abc123" || actual.Method != "POST" || !actual.Payload.Active {
		t.Fatalf("unexpected bound input: %+v", actual)
	}
	if !reflect.DeepEqual(actual.Names, []string{"Ada", "Grace"}) {
		t.Fatalf("Names = %#v", actual.Names)
	}
	if !reflect.DeepEqual(actual.Tokens, []string{"secret", "secondary"}) {
		t.Fatalf("Tokens = %#v", actual.Tokens)
	}
	if actual.Has == nil || !actual.Has.PathID || !actual.Has.Payload || actual.Payload.Has == nil || !actual.Payload.Has.Active {
		t.Fatalf("presence markers were not populated: input=%+v payload=%+v", actual.Has, actual.Payload.Has)
	}
}

func TestValueScopeExposesOnlySuppliedCanonicalDataPoints(t *testing.T) {
	body, err := bodyprovider.New([]byte(`{"name":"Ada"}`), "application/json", nil, bodyprovider.WithExactFieldNames())
	if err != nil {
		t.Fatal(err)
	}
	path := map[string]string{"id": "7"}
	query := url.Values{"tag": {"a", "b"}}
	headers := http.Header{"X-Mode": {"fast"}}
	cookies := map[string]string{"session": "abc"}
	form := url.Values{"label": {"x", "y"}}
	scope := requestprovider.NewValues(
		requestprovider.WithPathParams(path),
		requestprovider.WithQuery(query),
		requestprovider.WithHeaders(headers),
		requestprovider.WithCookies(cookies),
		requestprovider.WithForm(form),
		requestprovider.WithBodySource(body),
	)
	path["id"] = "99"
	query["tag"][0] = "changed"
	headers.Set("X-Mode", "changed")
	cookies["session"] = "changed"
	form["label"][0] = "changed"
	wantKinds := []string{requestprovider.QueryKind, requestprovider.PathKind, requestprovider.HeaderKind, requestprovider.CookieKind, requestprovider.FormKind, requestprovider.BodyKind}
	providers := scope.Providers()
	if len(providers) != len(wantKinds) {
		t.Fatalf("providers = %d, want %d", len(providers), len(wantKinds))
	}
	for index, provider := range providers {
		if provider.Kind() != wantKinds[index] {
			t.Fatalf("provider %d kind = %q, want %q", index, provider.Kind(), wantKinds[index])
		}
	}
	assertProviderValue(t, scope.Path(), reflect.TypeOf(0), "id", "7")
	assertProviderValue(t, scope.Query(), reflect.TypeOf([]string{}), "tag", []string{"a", "b"})
	assertProviderValue(t, scope.Header(), reflect.TypeOf(""), "x-mode", "fast")
	assertProviderValue(t, scope.Cookie(), reflect.TypeOf(""), "session", "abc")
	assertProviderValue(t, scope.Form(), reflect.TypeOf([]string{}), "label", []string{"x", "y"})
	assertProviderValue(t, scope.Body(), reflect.TypeOf(""), "name", "Ada")

	overlay := scope.WithHeader("Authorization", "Bearer token")
	assertProviderValue(t, overlay.Header(), reflect.TypeOf(""), "authorization", "Bearer token")
	if value, found, err := scope.Header().Locate(nil).Value(context.Background(), reflect.TypeOf(""), "authorization"); err != nil || found || value != nil {
		t.Fatalf("original header scope mutated: value=%v found=%v err=%v", value, found, err)
	}
}

func assertProviderValue(t *testing.T, provider locator.Provider, targetType reflect.Type, name string, want interface{}) {
	t.Helper()
	value, found, err := provider.Locate(nil).Value(context.Background(), targetType, name)
	if err != nil || !found || !reflect.DeepEqual(value, want) {
		t.Fatalf("value %q = %#v, found=%v, err=%v; want %#v", name, value, found, err, want)
	}
}

func TestRepeatedRequestValuesUseFirstForScalarAndAllForCollections(t *testing.T) {
	type input struct {
		QueryScalar int
		QuerySlice  []int
		FormScalar  string
		FormArray   [2]string
	}
	root, err := bindly.NewInjector()
	if err != nil {
		t.Fatalf("NewInjector() error = %v", err)
	}
	plan, err := root.CompilePlan(reflect.TypeOf(input{}),
		bindly.BindingSpec{Path: "QueryScalar", Location: state.Location{Kind: requestprovider.QueryKind, In: "id"}},
		bindly.BindingSpec{Path: "QuerySlice", Location: state.Location{Kind: requestprovider.QueryKind, In: "id"}},
		bindly.BindingSpec{Path: "FormScalar", Location: state.Location{Kind: requestprovider.FormKind, In: "name"}},
		bindly.BindingSpec{Path: "FormArray", Location: state.Location{Kind: requestprovider.FormKind, In: "name"}},
	)
	if err != nil {
		t.Fatalf("CompilePlan() error = %v", err)
	}
	values := url.Values{"name": {"Ada", "Grace"}}
	req := httptest.NewRequest("POST", "/?id=7&id=8", bytes.NewBufferString(values.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	scope, err := requestprovider.New(req)
	if err != nil {
		t.Fatalf("request.New() error = %v", err)
	}
	injector, err := root.ForScope(scope.Providers()...)
	if err != nil {
		t.Fatalf("ForScope() error = %v", err)
	}
	actual := &input{}
	if err := injector.Bind(context.Background(), actual, bindly.WithPlan(plan)); err != nil {
		t.Fatalf("Bind() error = %v", err)
	}
	if actual.QueryScalar != 7 || !reflect.DeepEqual(actual.QuerySlice, []int{7, 8}) || actual.FormScalar != "Ada" || actual.FormArray != [2]string{"Ada", "Grace"} {
		t.Fatalf("input = %+v", actual)
	}
}

func TestRepeatedRequestValuesRejectArrayOverflowWithoutPanic(t *testing.T) {
	type input struct {
		IDs [1]int
	}
	root, err := bindly.NewInjector()
	if err != nil {
		t.Fatalf("NewInjector() error = %v", err)
	}
	plan, err := root.CompilePlan(reflect.TypeOf(input{}), bindly.BindingSpec{
		Path: "IDs", Location: state.Location{Kind: requestprovider.QueryKind, In: "id"},
	})
	if err != nil {
		t.Fatalf("CompilePlan() error = %v", err)
	}
	scope, err := requestprovider.New(httptest.NewRequest("GET", "/?id=7&id=8", nil))
	if err != nil {
		t.Fatalf("request.New() error = %v", err)
	}
	injector, err := root.ForScope(scope.Providers()...)
	if err != nil {
		t.Fatalf("ForScope() error = %v", err)
	}
	err = injector.Bind(context.Background(), &input{}, bindly.WithPlan(plan))
	var invalid *inputerror.Error
	if err == nil || !errors.As(err, &invalid) || !strings.Contains(invalid.Cause.Error(), "array length mismatch") {
		t.Fatalf("Bind() error = %v", err)
	}
}

func TestFormProviderSupportsURLEncodedValues(t *testing.T) {
	type input struct {
		Names []string
		Age   int
	}
	root, err := bindly.NewInjector()
	if err != nil {
		t.Fatalf("NewInjector() error = %v", err)
	}
	plan, err := root.CompilePlan(reflect.TypeOf(input{}),
		bindly.BindingSpec{Path: "Names", Location: state.Location{Kind: requestprovider.FormKind, In: "name"}},
		bindly.BindingSpec{Path: "Age", Location: state.Location{Kind: requestprovider.FormKind, In: "age"}},
	)
	if err != nil {
		t.Fatalf("CompilePlan() error = %v", err)
	}
	values := url.Values{"name": {"Ada", "Grace"}, "age": {"42"}}
	req := httptest.NewRequest("POST", "/", bytes.NewBufferString(values.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	scope, err := requestprovider.New(req)
	if err != nil {
		t.Fatalf("request.New() error = %v", err)
	}
	injector, err := root.ForScope(scope.Providers()...)
	if err != nil {
		t.Fatalf("ForScope() error = %v", err)
	}
	actual := &input{}
	if err := injector.Bind(context.Background(), actual, bindly.WithPlan(plan)); err != nil {
		t.Fatalf("Bind() error = %v", err)
	}
	if !reflect.DeepEqual(actual.Names, []string{"Ada", "Grace"}) || actual.Age != 42 {
		t.Fatalf("unexpected form input: %+v", actual)
	}
}

func TestBodyProviderSupportsNamedJSONAndPluggableDecoder(t *testing.T) {
	type input struct {
		Name string
		XML  xmlPayload
	}
	root, err := bindly.NewInjector()
	if err != nil {
		t.Fatalf("NewInjector() error = %v", err)
	}
	jsonPlan, err := root.CompilePlan(reflect.TypeOf(input{}), bindly.BindingSpec{
		Path: "Name", Location: state.Location{Kind: requestprovider.BodyKind, In: "name"},
	})
	if err != nil {
		t.Fatalf("CompilePlan(JSON) error = %v", err)
	}
	jsonReq := httptest.NewRequest("POST", "/", bytes.NewBufferString(`{"name":"Ada"}`))
	jsonReq.Header.Set("Content-Type", "application/json")
	jsonScope, err := requestprovider.New(jsonReq)
	if err != nil {
		t.Fatalf("request.New(JSON) error = %v", err)
	}
	jsonInjector, err := root.ForScope(jsonScope.Providers()...)
	if err != nil {
		t.Fatalf("ForScope(JSON) error = %v", err)
	}
	jsonInput := &input{}
	if err := jsonInjector.Bind(context.Background(), jsonInput, bindly.WithPlan(jsonPlan)); err != nil {
		t.Fatalf("JSON Bind() error = %v", err)
	}
	if jsonInput.Name != "Ada" {
		t.Fatalf("Name = %q", jsonInput.Name)
	}

	xmlPlan, err := root.CompilePlan(reflect.TypeOf(input{}), bindly.BindingSpec{
		Path: "XML", Location: state.Location{Kind: requestprovider.BodyKind},
	})
	if err != nil {
		t.Fatalf("CompilePlan(XML) error = %v", err)
	}
	xmlReq := httptest.NewRequest("POST", "/", bytes.NewBufferString(`<payload><name>Grace</name></payload>`))
	xmlReq.Header.Set("Content-Type", "application/xml")
	xmlScope, err := requestprovider.New(xmlReq, requestprovider.WithDecoder("application/xml", requestprovider.DecoderFunc(func(data []byte, target interface{}) error {
		return xml.Unmarshal(data, target)
	})))
	if err != nil {
		t.Fatalf("request.New(XML) error = %v", err)
	}
	xmlInjector, err := root.ForScope(xmlScope.Providers()...)
	if err != nil {
		t.Fatalf("ForScope(XML) error = %v", err)
	}
	xmlInput := &input{}
	if err := xmlInjector.Bind(context.Background(), xmlInput, bindly.WithPlan(xmlPlan)); err != nil {
		t.Fatalf("XML Bind() error = %v", err)
	}
	if xmlInput.XML.Name != "Grace" {
		t.Fatalf("XML.Name = %q", xmlInput.XML.Name)
	}
}

func TestBodyAndFormProvidersSupportMultipartValuesAndFiles(t *testing.T) {
	type input struct {
		Names []string
		Name  string
		File  *multipart.FileHeader
	}
	root, err := bindly.NewInjector()
	if err != nil {
		t.Fatalf("NewInjector() error = %v", err)
	}
	for _, kind := range []string{requestprovider.BodyKind, requestprovider.FormKind} {
		t.Run(kind, func(t *testing.T) {
			plan, err := root.CompilePlan(reflect.TypeOf(input{}),
				bindly.BindingSpec{Path: "Names", Location: state.Location{Kind: kind, In: "name"}},
				bindly.BindingSpec{Path: "Name", Location: state.Location{Kind: kind, In: "name"}},
				bindly.BindingSpec{Path: "File", Location: state.Location{Kind: kind, In: "upload"}},
			)
			if err != nil {
				t.Fatalf("CompilePlan() error = %v", err)
			}
			req := multipartRequest(t)
			scope, err := requestprovider.New(req)
			if err != nil {
				t.Fatalf("request.New() error = %v", err)
			}
			if kind == requestprovider.FormKind {
				value, ok, err := scope.Form().Locate(nil).Value(context.Background(), nil, "name")
				if err != nil || !ok || !reflect.DeepEqual(value, []string{"Ada", "Grace"}) {
					t.Fatalf("untyped form Value() = (%#v, %v, %v)", value, ok, err)
				}
			}
			t.Cleanup(func() {
				if err := scope.Close(); err != nil {
					t.Errorf("scope.Close() error = %v", err)
				}
			})
			injector, err := root.ForScope(scope.Providers()...)
			if err != nil {
				t.Fatalf("ForScope() error = %v", err)
			}
			actual := &input{}
			if err := injector.Bind(context.Background(), actual, bindly.WithPlan(plan)); err != nil {
				t.Fatalf("Bind() error = %v", err)
			}
			if !reflect.DeepEqual(actual.Names, []string{"Ada", "Grace"}) {
				t.Fatalf("Names = %#v", actual.Names)
			}
			if actual.Name != "Ada" {
				t.Fatalf("Name = %q", actual.Name)
			}
			if actual.File == nil || actual.File.Filename != "sample.txt" {
				t.Fatalf("File = %+v", actual.File)
			}
		})
	}
}

func multipartRequest(t *testing.T) *http.Request {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	if err := writer.WriteField("name", "Ada"); err != nil {
		t.Fatalf("WriteField(Ada) error = %v", err)
	}
	if err := writer.WriteField("name", "Grace"); err != nil {
		t.Fatalf("WriteField(Grace) error = %v", err)
	}
	part, err := writer.CreateFormFile("upload", "sample.txt")
	if err != nil {
		t.Fatalf("CreateFormFile() error = %v", err)
	}
	if _, err := io.WriteString(part, "content"); err != nil {
		t.Fatalf("WriteString() error = %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("multipart.Close() error = %v", err)
	}
	req := httptest.NewRequest("POST", "/", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}

type xmlPayload struct {
	XMLName xml.Name `xml:"payload"`
	Name    string   `xml:"name"`
}
