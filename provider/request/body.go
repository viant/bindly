package request

import (
	"bytes"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"reflect"
	"strings"
	"sync"

	bodyprovider "github.com/viant/bindly/provider/body"
)

var fileHeaderType = reflect.TypeOf((*multipart.FileHeader)(nil))

type bodySource struct {
	scope  *Scope
	source *bodyprovider.Source
	once   sync.Once
	raw    []byte
	err    error
}

func (b *bodySource) value(targetType reflect.Type, name string) (interface{}, bool, error) {
	if targetType == nil {
		return nil, false, fmt.Errorf("body binding target type is required")
	}
	if b.source != nil {
		return b.source.Value(targetType, name)
	}
	mediaType, err := b.mediaType()
	if err != nil {
		return nil, false, err
	}
	if strings.HasPrefix(mediaType, "multipart/") {
		return b.multipartValue(targetType, name)
	}
	b.read()
	if b.err != nil {
		return nil, false, b.err
	}
	if len(b.raw) == 0 {
		return nil, false, nil
	}
	if name == "" {
		switch {
		case targetType == reflect.TypeOf([]byte{}):
			return append([]byte(nil), b.raw...), true, nil
		case targetType.Kind() == reflect.String:
			return string(b.raw), true, nil
		}
	}
	source, err := bodyprovider.New(b.raw, mediaType, b.scope.decoders)
	if err != nil {
		return nil, false, err
	}
	return source.Value(targetType, name)
}

func (b *bodySource) mediaType() (string, error) {
	contentType := ""
	if b.scope != nil && b.scope.request != nil {
		contentType = b.scope.request.Header.Get("Content-Type")
	}
	if strings.TrimSpace(contentType) == "" {
		return "application/json", nil
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return "", fmt.Errorf("parse request content type %q: %w", contentType, err)
	}
	return strings.ToLower(mediaType), nil
}

func (b *bodySource) read() {
	b.once.Do(func() {
		if b.scope == nil || b.scope.request == nil || b.scope.request.Body == nil {
			return
		}
		b.raw, b.err = io.ReadAll(b.scope.request.Body)
		if b.err == nil {
			b.scope.request.Body = io.NopCloser(bytes.NewReader(b.raw))
		}
	})
}

func (b *bodySource) multipartValue(targetType reflect.Type, name string) (interface{}, bool, error) {
	if name == "" {
		return nil, false, fmt.Errorf("multipart body binding requires a field name")
	}
	request := b.scope.request
	if request == nil {
		return nil, false, nil
	}
	if err := b.scope.parseForm(); err != nil {
		return nil, false, fmt.Errorf("parse multipart request body: %w", err)
	}
	if request.MultipartForm == nil {
		return nil, false, nil
	}
	return multipartValue(request.MultipartForm, targetType, name)
}

func multipartValue(form *multipart.Form, targetType reflect.Type, name string) (interface{}, bool, error) {
	if targetType == fileHeaderType {
		files := form.File[name]
		if len(files) == 0 {
			return nil, false, nil
		}
		return files[0], true, nil
	}
	if targetType != nil && targetType.Kind() == reflect.Slice && targetType.Elem() == fileHeaderType {
		files := form.File[name]
		if len(files) == 0 {
			return nil, false, nil
		}
		return files, true, nil
	}
	return requestValues(form.Value[name], targetType)
}
