package request

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"strings"

	"github.com/viant/bindly/locator"
	"github.com/viant/structology"
)

type RequestProvider struct{ scope *Scope }
type QueryProvider struct{ scope *Scope }
type PathProvider struct{ scope *Scope }
type HeaderProvider struct{ scope *Scope }
type CookieProvider struct{ scope *Scope }
type FormProvider struct{ scope *Scope }
type BodyProvider struct{ scope *Scope }

func (p *RequestProvider) Kind() string           { return RequestKind }
func (p *QueryProvider) Kind() string             { return QueryKind }
func (p *PathProvider) Kind() string              { return PathKind }
func (p *HeaderProvider) Kind() string            { return HeaderKind }
func (p *CookieProvider) Kind() string            { return CookieKind }
func (p *FormProvider) Kind() string              { return FormKind }
func (p *BodyProvider) Kind() string              { return BodyKind }
func (p *RequestProvider) Priority() int          { return locator.PrioritySource }
func (p *QueryProvider) Priority() int            { return locator.PrioritySource }
func (p *PathProvider) Priority() int             { return locator.PrioritySource }
func (p *HeaderProvider) Priority() int           { return locator.PrioritySource }
func (p *CookieProvider) Priority() int           { return locator.PrioritySource }
func (p *FormProvider) Priority() int             { return locator.PrioritySource }
func (p *BodyProvider) Priority() int             { return locator.PrioritySource }
func (p *RequestProvider) DefaultCacheable() bool { return true }
func (p *QueryProvider) DefaultCacheable() bool   { return true }
func (p *PathProvider) DefaultCacheable() bool    { return true }
func (p *HeaderProvider) DefaultCacheable() bool  { return true }
func (p *CookieProvider) DefaultCacheable() bool  { return true }
func (p *FormProvider) DefaultCacheable() bool    { return true }
func (p *BodyProvider) DefaultCacheable() bool    { return true }

func (p *RequestProvider) Locate(*structology.State) locator.Locator {
	return &requestLocator{scope: p.scope}
}
func (p *QueryProvider) Locate(*structology.State) locator.Locator {
	return &queryLocator{scope: p.scope}
}
func (p *PathProvider) Locate(*structology.State) locator.Locator {
	return &pathLocator{scope: p.scope}
}
func (p *HeaderProvider) Locate(*structology.State) locator.Locator {
	return &headerLocator{scope: p.scope}
}
func (p *CookieProvider) Locate(*structology.State) locator.Locator {
	return &cookieLocator{scope: p.scope}
}
func (p *FormProvider) Locate(*structology.State) locator.Locator {
	return &formLocator{scope: p.scope}
}
func (p *BodyProvider) Locate(*structology.State) locator.Locator {
	return &bodyLocator{scope: p.scope}
}

type requestLocator struct{ scope *Scope }
type queryLocator struct{ scope *Scope }
type pathLocator struct{ scope *Scope }
type headerLocator struct{ scope *Scope }
type cookieLocator struct{ scope *Scope }
type formLocator struct{ scope *Scope }
type bodyLocator struct{ scope *Scope }

func (*requestLocator) Kind() string { return RequestKind }
func (*queryLocator) Kind() string   { return QueryKind }
func (*pathLocator) Kind() string    { return PathKind }
func (*headerLocator) Kind() string  { return HeaderKind }
func (*cookieLocator) Kind() string  { return CookieKind }
func (*formLocator) Kind() string    { return FormKind }
func (*bodyLocator) Kind() string    { return BodyKind }

func (l *requestLocator) Value(_ context.Context, targetType reflect.Type, name string) (interface{}, bool, error) {
	if l.scope == nil || l.scope.request == nil {
		return nil, false, nil
	}
	request := l.scope.request
	switch strings.ToLower(name) {
	case "uri":
		return request.RequestURI, true, nil
	case "header":
		return request.Header, true, nil
	case "proto":
		return request.Proto, true, nil
	case "remoteaddr":
		return request.RemoteAddr, true, nil
	case "host":
		return request.Host, true, nil
	case "method":
		return request.Method, true, nil
	}
	if targetType != nil && !reflect.TypeOf(l.scope.request).AssignableTo(targetType) {
		return nil, false, fmt.Errorf("request value cannot bind to %v", targetType)
	}
	return l.scope.request, true, nil
}

func (l *queryLocator) Value(_ context.Context, targetType reflect.Type, name string) (interface{}, bool, error) {
	if l.scope == nil {
		return nil, false, nil
	}
	values := l.scope.queryValues()
	if name == "" {
		if l.scope.query != nil {
			return values.Encode(), true, nil
		}
		if l.scope.request != nil && l.scope.request.URL != nil {
			return l.scope.request.URL.RawQuery, true, nil
		}
		return "", true, nil
	}
	return requestValues(values[name], targetType)
}

func (l *pathLocator) Value(_ context.Context, _ reflect.Type, name string) (interface{}, bool, error) {
	if l.scope == nil {
		return nil, false, nil
	}
	if name == "" {
		if l.scope.request != nil && l.scope.request.URL != nil {
			return l.scope.request.URL.Path, true, nil
		}
		return "", true, nil
	}
	value, ok := l.scope.path[name]
	return value, ok, nil
}

func (l *headerLocator) Value(_ context.Context, targetType reflect.Type, name string) (interface{}, bool, error) {
	if l.scope == nil || name == "" {
		return nil, false, nil
	}
	values := l.scope.headerValues().Values(name)
	return requestValues(values, targetType)
}

func (l *cookieLocator) Value(_ context.Context, _ reflect.Type, name string) (interface{}, bool, error) {
	if l.scope == nil || name == "" {
		return nil, false, nil
	}
	if l.scope.cookies != nil {
		value, ok := l.scope.cookies[name]
		return value, ok, nil
	}
	if l.scope.request == nil {
		return nil, false, nil
	}
	cookie, err := l.scope.request.Cookie(name)
	if err == http.ErrNoCookie {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return cookie.Value, true, nil
}

func (l *formLocator) Value(_ context.Context, targetType reflect.Type, name string) (interface{}, bool, error) {
	if l.scope == nil || name == "" {
		return nil, false, nil
	}
	if l.scope.form != nil {
		return requestValues(l.scope.form[name], targetType)
	}
	if l.scope.request == nil {
		return nil, false, nil
	}
	if err := l.scope.parseForm(); err != nil {
		return nil, false, fmt.Errorf("parse request form: %w", err)
	}
	request := l.scope.request
	if request.MultipartForm != nil {
		if value, ok, err := multipartValue(request.MultipartForm, targetType, name); ok || err != nil {
			return value, ok, err
		}
	}
	return requestValues(request.Form[name], targetType)
}

func (l *bodyLocator) Value(_ context.Context, targetType reflect.Type, name string) (interface{}, bool, error) {
	if l.scope == nil {
		return nil, false, nil
	}
	return l.scope.body.value(targetType, name)
}

func requestValues(values []string, targetType reflect.Type) (interface{}, bool, error) {
	switch len(values) {
	case 0:
		return nil, false, nil
	case 1:
		return values[0], true, nil
	default:
		if targetType != nil && !destinationIsCollection(targetType) {
			return values[0], true, nil
		}
		return values, true, nil
	}
}

func destinationIsCollection(targetType reflect.Type) bool {
	for targetType != nil && targetType.Kind() == reflect.Ptr {
		targetType = targetType.Elem()
	}
	return targetType != nil && (targetType.Kind() == reflect.Slice || targetType.Kind() == reflect.Array)
}
