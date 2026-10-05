// Package request composes independently scoped HTTP value providers while
// preserving the original request pointer for handler capabilities.
package request

import (
	"bytes"
	"context"
	"fmt"
	"github.com/viant/bindly/input"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"sync"

	"github.com/viant/bindly/locator"
	"github.com/viant/bindly/provider/body"
	"github.com/viant/structology"
)

const (
	QueryKind       = "query"
	PathKind        = "path"
	HeaderKind      = "header"
	CookieKind      = "cookie"
	FormKind        = "form"
	BodyKind        = "body"
	HTTPRequestKind = "http_request"
	RequestKind     = HTTPRequestKind
)

type Scope struct {
	request            *http.Request
	path               map[string]string
	query              url.Values
	headers            http.Header
	cookies            map[string]string
	form               url.Values
	body               *body.Source
	cleanup            *multipartCleanup
	ignoreEmptyQuery   bool
	decoders           *DecoderRegistry
	maxMultipartMemory int64
	multipart          *multipart.Form
	formOverlay        bool
	queryOverlay       bool
}

type multipartCleanup struct {
	once sync.Once
	form *multipart.Form
	err  error
}

func (s *Scope) Close() error {
	if s == nil || s.cleanup == nil {
		return nil
	}
	s.cleanup.once.Do(func() { s.cleanup.err = s.cleanup.form.RemoveAll() })
	return s.cleanup.err
}

type Option func(*Scope)

func WithDecoder(mediaType string, decoder Decoder) Option {
	return func(s *Scope) {
		if s.decoders == nil {
			s.decoders = NewDecoderRegistry()
		}
		s.decoders.Register(mediaType, decoder)
	}
}
func WithMaxMultipartMemory(size int64) Option {
	return func(s *Scope) {
		if size > 0 {
			s.maxMultipartMemory = size
		}
	}
}

func WithPathParams(values map[string]string) Option {
	return func(s *Scope) { s.path = copyStrings(values) }
}
func WithQuery(values url.Values) Option {
	return func(s *Scope) { s.query = copyValues(values); s.queryOverlay = true }
}

// WithIgnoreEmptyQueryParameters treats a single empty query value as absent.
// Repeated values and other provider kinds retain their authored presence.
func WithIgnoreEmptyQueryParameters(ignore bool) Option {
	return func(s *Scope) { s.ignoreEmptyQuery = ignore }
}
func WithHeaders(values http.Header) Option { return func(s *Scope) { s.headers = values.Clone() } }
func WithCookies(values map[string]string) Option {
	return func(s *Scope) { s.cookies = copyStrings(values) }
}
func WithForm(values url.Values) Option {
	return func(s *Scope) { s.form = copyValues(values); s.formOverlay = true }
}
func WithBodySource(source *body.Source) Option { return func(s *Scope) { s.body = source } }
func NewValues(options ...Option) *Scope {
	s := &Scope{}
	for _, option := range options {
		if option != nil {
			option(s)
		}
	}
	return s
}

// WithHeader returns a detached header overlay. The underlying request pointer
// and all other source values retain their invocation identity.
func (s *Scope) WithHeader(name, value string) *Scope {
	result := &Scope{}
	if s != nil {
		*result = *s
		result.headers = s.headers.Clone()
	}
	if result.headers == nil {
		result.headers = make(http.Header)
	}
	result.headers.Set(name, value)
	return result
}
func New(request *http.Request, options ...Option) (*Scope, error) {
	if request == nil {
		return nil, fmt.Errorf("HTTP request is required")
	}
	s := &Scope{request: request, headers: request.Header.Clone(), cookies: map[string]string{}, maxMultipartMemory: 32 << 20}
	if request.URL != nil {
		s.query = request.URL.Query()
	}
	for _, cookie := range request.Cookies() {
		s.cookies[cookie.Name] = cookie.Value
	}
	for _, option := range options {
		if option != nil {
			option(s)
		}
	}
	var raw []byte
	if request.Body != nil {
		var err error
		raw, err = io.ReadAll(request.Body)
		if err != nil {
			return nil, err
		}
		request.Body = io.NopCloser(bytes.NewReader(raw))
	}
	var err error
	clone := request.Clone(request.Context())
	clone.Body = io.NopCloser(bytes.NewReader(raw))
	if err = clone.ParseForm(); err != nil {
		return nil, &input.Error{Cause: err}
	}
	mediaType, _, _ := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if mediaType == "multipart/form-data" {
		if err = clone.ParseMultipartForm(s.maxMultipartMemory); err != nil {
			return nil, &input.Error{Cause: err}
		}
		if clone.MultipartForm != nil && clone.MultipartForm != request.MultipartForm {
			s.cleanup = &multipartCleanup{form: clone.MultipartForm}
		}
	}
	s.multipart = clone.MultipartForm
	if s.form == nil {
		s.form = copyValues(clone.PostForm)
	}
	if s.body == nil {
		s.body, err = body.New(raw, request.Header.Get("Content-Type"), nil, body.WithDecoders(s.decoders), body.WithMultipartForm(s.multipart))
		if err != nil {
			_ = s.Close()
			return nil, err
		}
	}
	return s, nil
}

func (s *Scope) Providers() []locator.Provider {
	if s == nil {
		return nil
	}
	var result []locator.Provider
	if s.request != nil || s.query != nil {
		result = append(result, s.Query())
	}
	if s.request != nil || s.path != nil {
		result = append(result, s.Path())
	}
	if s.request != nil || s.headers != nil {
		result = append(result, s.Header())
	}
	if s.request != nil || s.cookies != nil {
		result = append(result, s.Cookie())
	}
	if s.request != nil || s.form != nil {
		result = append(result, s.Form())
	}
	if s.body != nil {
		result = append(result, s.body)
	}
	if s.request != nil {
		result = append(result, s.Request())
	}
	return result
}

func (s *Scope) Request() locator.Provider {
	return &source{kind: HTTPRequestKind, lookup: func(name string) (any, bool) {
		if s == nil || s.request == nil {
			return nil, false
		}
		name = strings.ToLower(name)
		switch name {
		case "":
			return s.request, true
		case "method":
			return s.request.Method, true
		case "uri":
			return s.request.RequestURI, true
		case "url":
			return s.request.URL, true
		case "header":
			return s.request.Header, true
		case "proto":
			return s.request.Proto, true
		case "remoteaddr":
			return s.request.RemoteAddr, true
		case "host":
			return s.request.Host, true
		}
		return s.request, true
	}}
}

type source struct {
	kind          string
	lookup        func(string) (any, bool)
	contextLookup func(context.Context, string) (any, bool)
	typedLookup   func(reflect.Type, string) (any, bool)
}

func (s *Scope) Query() locator.Provider {
	return &source{kind: QueryKind, contextLookup: func(ctx context.Context, name string) (any, bool) {
		if s == nil {
			return nil, false
		}
		if name == "" {
			if s.request != nil && s.request.URL != nil && !s.queryOverlay {
				return s.request.URL.RawQuery, true
			}
			return s.query.Encode(), true
		}
		value, ok := s.query[name]
		ignore := s.ignoreEmptyQuery
		if policy, ok := ctx.Value(queryPolicyKey{}).(QueryPolicy); ok {
			ignore = policy.IgnoreEmptyParameters
		}
		if ok && ignore && (len(value) == 0 || len(value) == 1 && value[0] == "") {
			return nil, false
		}
		return value, ok
	}}
}
func (s *Scope) Path() locator.Provider {
	return &source{kind: PathKind, lookup: func(name string) (any, bool) {
		if s == nil {
			return nil, false
		}
		if name == "" {
			if s.request != nil && s.request.URL != nil {
				return s.request.URL.Path, true
			}
			return "", true
		}
		value, ok := s.path[name]
		return value, ok
	}}
}
func (s *Scope) Header() locator.Provider {
	return &source{kind: HeaderKind, lookup: func(name string) (any, bool) {
		if s == nil {
			return nil, false
		}
		value, ok := s.headers[http.CanonicalHeaderKey(name)]
		return value, ok
	}}
}
func (s *Scope) Cookie() locator.Provider {
	return &source{kind: CookieKind, lookup: func(name string) (any, bool) {
		if s == nil {
			return nil, false
		}
		value, ok := s.cookies[name]
		return value, ok
	}}
}
func (s *Scope) Form() locator.Provider {
	return &source{kind: FormKind, typedLookup: func(target reflect.Type, name string) (any, bool) {
		if s == nil {
			return nil, false
		}
		if s.multipart != nil && !s.formOverlay {
			if value, found := body.MultipartValue(s.multipart, target, name); found {
				return value, true
			}
		}
		value, ok := s.form[name]
		return value, ok
	}}
}
func (s *Scope) Body() *body.Source {
	if s == nil {
		return nil
	}
	return s.body
}

func (s *source) Kind() string                              { return s.kind }
func (s *source) Priority() int                             { return 0 }
func (s *source) DefaultCacheable() bool                    { return true }
func (s *source) Locate(*structology.State) locator.Locator { return s }
func (s *source) Value(ctx context.Context, target reflect.Type, name string) (any, bool, error) {
	var value any
	var ok bool
	if s.typedLookup != nil {
		value, ok = s.typedLookup(target, name)
	} else if s.contextLookup != nil {
		value, ok = s.contextLookup(ctx, name)
	} else {
		value, ok = s.lookup(name)
	}
	if !ok {
		return nil, false, nil
	}
	for target != nil && target.Kind() == reflect.Pointer {
		target = target.Elem()
	}
	collection := target != nil && (target.Kind() == reflect.Slice || target.Kind() == reflect.Array) && target != reflect.TypeOf([]byte{})
	if values, ok := value.([]string); ok && !collection && target != nil && len(values) > 0 {
		return values[0], true, nil
	}
	return value, true, nil
}

func copyStrings(values map[string]string) map[string]string {
	result := make(map[string]string, len(values))
	for name, value := range values {
		result[name] = value
	}
	return result
}
func copyValues(values url.Values) url.Values {
	result := make(url.Values, len(values))
	for name, value := range values {
		result[name] = append([]string(nil), value...)
	}
	return result
}
