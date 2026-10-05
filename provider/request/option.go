package request

import (
	"net/http"
	"net/url"

	bodyprovider "github.com/viant/bindly/provider/body"
)

type Option func(*Scope)

func WithPathParams(params map[string]string) Option {
	return func(scope *Scope) {
		scope.path = clonePath(params)
	}
}

func WithQuery(query url.Values) Option {
	return func(scope *Scope) {
		scope.query = cloneValues(query)
	}
}

func WithHeaders(headers http.Header) Option {
	return func(scope *Scope) {
		scope.headers = headers.Clone()
	}
}

func WithCookies(cookies map[string]string) Option {
	return func(scope *Scope) {
		scope.cookies = clonePath(cookies)
	}
}

func WithForm(form url.Values) Option {
	return func(scope *Scope) {
		scope.form = cloneValues(form)
	}
}

// WithBodySource supplies an already validated protocol-neutral body source.
func WithBodySource(source *bodyprovider.Source) Option {
	return func(scope *Scope) {
		scope.body.source = source
	}
}

func WithDecoder(mediaType string, decoder Decoder) Option {
	return func(scope *Scope) {
		scope.decoders.Register(mediaType, decoder)
	}
}

func WithMaxMultipartMemory(bytes int64) Option {
	return func(scope *Scope) {
		if bytes > 0 {
			scope.maxMultipartMemory = bytes
		}
	}
}
