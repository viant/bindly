package request

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"sync"

	"github.com/viant/bindly/input"
	"github.com/viant/bindly/provider/body"
)

// NewDeferred opts this invocation into demand-driven body/form initialization.
// New retains its eager construction and option timing. Metadata providers do
// not read bytes; the full raw-request capability snapshots and rewinds them.
func NewDeferred(request *http.Request, options ...Option) (*Scope, error) {
	if request == nil {
		return nil, fmt.Errorf("HTTP request is required")
	}
	d := &deferredRequest{request: request}
	s := &Scope{request: request, deferred: d, headers: request.Header.Clone(), cookies: map[string]string{}, maxMultipartMemory: 32 << 20}
	if request.URL != nil {
		s.query = request.URL.Query()
	}
	for _, cookie := range request.Cookies() {
		s.cookies[cookie.Name] = cookie.Value
	}
	s.body = body.NewDeferred(func(ctx context.Context) (*body.Source, error) {
		raw, err := d.bytes(ctx)
		if err != nil {
			return nil, err
		}
		contentType := request.Header.Get("Content-Type")
		mediaType, _, _ := mime.ParseMediaType(contentType)
		var form *multipart.Form
		if mediaType == "multipart/form-data" {
			_, form, err = d.formValues(ctx)
			if err != nil {
				return nil, err
			}
		}
		return body.New(raw, contentType, nil, body.WithDecoders(s.decoders), body.WithMultipartForm(form))
	})
	for _, option := range options {
		if option != nil {
			option(s)
		}
	}
	d.maxMultipartMemory = s.maxMultipartMemory
	return s, nil
}

type deferredRequest struct {
	maxMultipartMemory int64
	multipart          *multipart.Form
	mu                 sync.Mutex
	request            *http.Request
	rawReady           bool
	raw                []byte
	rawErr             error
	formReady          bool
	form               url.Values
	formErr            error
	cleanup            *multipart.Form
	closed             bool
	closeErr           error
}

func (d *deferredRequest) bytes(ctx context.Context) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.readLocked(ctx)
}

func (d *deferredRequest) readLocked(ctx context.Context) ([]byte, error) {
	if d.closed {
		return nil, fmt.Errorf("request source is closed")
	}
	if d.rawReady {
		return d.raw, d.rawErr
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	d.rawReady = true
	if d.request.Body != nil {
		d.raw, d.rawErr = io.ReadAll(d.request.Body)
		// Preserve even a partial snapshot; never retry an exhausted stream.
		d.request.Body = io.NopCloser(bytes.NewReader(d.raw))
	}
	if d.rawErr == nil {
		d.rawErr = ctx.Err()
	}
	return d.raw, d.rawErr
}

func (d *deferredRequest) formValues(ctx context.Context) (url.Values, *multipart.Form, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil, nil, fmt.Errorf("request source is closed")
	}
	if d.formReady {
		return d.form, d.multipart, d.formErr
	}
	raw, err := d.readLocked(ctx)
	if err != nil {
		return nil, nil, err
	}
	d.formReady = true
	clone := d.request.Clone(ctx)
	// Request.Clone copies multipart metadata but not its spill-file ownership.
	// Already-parsed caller forms remain owned by the original request.
	if d.request.MultipartForm != nil {
		clone.MultipartForm = d.request.MultipartForm
	}
	clone.Body = io.NopCloser(bytes.NewReader(raw))
	if err = clone.ParseForm(); err == nil {
		mediaType, _, _ := mime.ParseMediaType(d.request.Header.Get("Content-Type"))
		if mediaType == "multipart/form-data" {
			err = clone.ParseMultipartForm(d.maxMultipartMemory)
		}
	}
	// ParseMultipartForm can allocate spill files before reporting a failure.
	if clone.MultipartForm != nil && clone.MultipartForm != d.request.MultipartForm {
		d.cleanup = clone.MultipartForm
	}
	if err != nil {
		d.formErr = &input.Error{Cause: err}
		return nil, nil, d.formErr
	}
	if err = ctx.Err(); err != nil {
		d.formErr = err
		return nil, nil, err
	}
	d.multipart = clone.MultipartForm
	d.form = copyValues(clone.PostForm)
	return d.form, d.multipart, nil
}

func (d *deferredRequest) close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return d.closeErr
	}
	d.closed = true
	if d.cleanup != nil {
		d.closeErr = d.cleanup.RemoveAll()
	}
	return d.closeErr
}
