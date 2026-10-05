// Package request provides invocation-scoped HTTP request providers for
// Bindly. Packaging groups one concern; each request data point remains an
// independent binding kind.
package request

import (
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/viant/bindly/locator"
)

const (
	RequestKind = "http_request"
	QueryKind   = "query"
	PathKind    = "path"
	HeaderKind  = "header"
	CookieKind  = "cookie"
	FormKind    = "form"
	BodyKind    = "body"
)

// Scope groups providers derived from one HTTP request. Explicit overlays are
// useful for routers and non-HTTP test callers and win over request values.
type Scope struct {
	request            *http.Request
	path               map[string]string
	query              url.Values
	headers            http.Header
	cookies            map[string]string
	form               url.Values
	decoders           *DecoderRegistry
	maxMultipartMemory int64
	formOnce           sync.Once
	formErr            error
	body               bodySource
}

func New(req *http.Request, options ...Option) (*Scope, error) {
	if req == nil {
		return nil, fmt.Errorf("HTTP request is required")
	}
	return newScope(req, options...), nil
}

// NewValues creates a request-shaped binding scope without manufacturing an
// HTTP request. Protocol adapters use options to supply only the canonical
// request data points they actually own.
func NewValues(options ...Option) *Scope {
	return newScope(nil, options...)
}

func newScope(req *http.Request, options ...Option) *Scope {
	scope := &Scope{
		request:            req,
		decoders:           NewDecoderRegistry(),
		maxMultipartMemory: 32 << 20,
	}
	for _, option := range options {
		if option != nil {
			option(scope)
		}
	}
	scope.body.scope = scope
	return scope
}

// Providers returns every canonical request provider in deterministic order.
func (s *Scope) Providers() []locator.Provider {
	if s == nil {
		return nil
	}
	if s.request == nil {
		result := make([]locator.Provider, 0, 6)
		if s.query != nil {
			result = append(result, s.Query())
		}
		if s.path != nil {
			result = append(result, s.Path())
		}
		if s.headers != nil {
			result = append(result, s.Header())
		}
		if s.cookies != nil {
			result = append(result, s.Cookie())
		}
		if s.form != nil {
			result = append(result, s.Form())
		}
		if s.body.source != nil {
			result = append(result, s.Body())
		}
		return result
	}
	return []locator.Provider{s.Request(), s.Query(), s.Path(), s.Header(), s.Cookie(), s.Form(), s.Body()}
}

// WithHeader returns a detached scope with one header overlay.
func (s *Scope) WithHeader(name, value string) *Scope {
	result := newScope(nil)
	if s != nil {
		result.request = s.request
		result.path = clonePath(s.path)
		result.query = cloneValues(s.query)
		result.headers = s.headers.Clone()
		result.cookies = clonePath(s.cookies)
		result.form = cloneValues(s.form)
		result.decoders = s.decoders
		result.maxMultipartMemory = s.maxMultipartMemory
		result.body.source = s.body.source
		result.body.scope = result
	}
	if result.headers == nil {
		result.headers = make(http.Header)
	}
	result.headers.Set(name, value)
	return result
}

func clonePath(source map[string]string) map[string]string {
	if source == nil {
		return nil
	}
	result := make(map[string]string, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func cloneValues(source url.Values) url.Values {
	if source == nil {
		return nil
	}
	result := make(url.Values, len(source))
	for key, values := range source {
		result[key] = append([]string(nil), values...)
	}
	return result
}

func (s *Scope) Request() locator.Provider { return &RequestProvider{scope: s} }
func (s *Scope) Query() locator.Provider   { return &QueryProvider{scope: s} }
func (s *Scope) Path() locator.Provider    { return &PathProvider{scope: s} }
func (s *Scope) Header() locator.Provider  { return &HeaderProvider{scope: s} }
func (s *Scope) Cookie() locator.Provider  { return &CookieProvider{scope: s} }
func (s *Scope) Form() locator.Provider    { return &FormProvider{scope: s} }
func (s *Scope) Body() locator.Provider    { return &BodyProvider{scope: s} }

// Close removes multipart temporary files created while resolving body fields.
func (s *Scope) Close() error {
	if s == nil || s.request == nil || s.request.MultipartForm == nil {
		return nil
	}
	return s.request.MultipartForm.RemoveAll()
}

func (s *Scope) queryValues() url.Values {
	if s.query != nil {
		return s.query
	}
	if s.request == nil || s.request.URL == nil {
		return nil
	}
	return s.request.URL.Query()
}

func (s *Scope) headerValues() http.Header {
	if s.headers != nil {
		return s.headers
	}
	if s.request == nil {
		return nil
	}
	return s.request.Header
}

func (s *Scope) parseForm() error {
	if s == nil || s.request == nil {
		return nil
	}
	s.formOnce.Do(func() {
		contentType := s.request.Header.Get("Content-Type")
		mediaType, _, err := mime.ParseMediaType(contentType)
		if err == nil && strings.HasPrefix(strings.ToLower(mediaType), "multipart/") {
			s.formErr = s.request.ParseMultipartForm(s.maxMultipartMemory)
			return
		}
		s.formErr = s.request.ParseForm()
	})
	return s.formErr
}
