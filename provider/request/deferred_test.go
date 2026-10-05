package request

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
)

type demandReader struct {
	payload      string
	reads, bytes int
	fail         error
}

func (r *demandReader) Read(p []byte) (int, error) {
	r.reads++
	if r.payload == "" {
		if r.fail != nil {
			return 0, r.fail
		}
		return 0, io.EOF
	}
	n := copy(p, r.payload)
	r.payload = r.payload[n:]
	r.bytes += n
	if r.fail != nil {
		return n, r.fail
	}
	return n, nil
}
func (r *demandReader) Close() error { return nil }
func deferredValue(t *testing.T, s *Scope, kind, name string, target reflect.Type) (any, bool, error) {
	t.Helper()
	for _, p := range s.Providers() {
		if p.Kind() == kind {
			return p.Locate(nil).Value(context.Background(), target, name)
		}
	}
	t.Fatalf("missing provider %s", kind)
	return nil, false, nil
}

func TestDeferredRequestUnusedTransportZeroReads(t *testing.T) {
	for _, tc := range []struct{ name, contentType, payload string }{
		{"malformed JSON", "application/json", "{"},
		{"invalid MIME", "application/json; broken", "{}"},
		{"missing boundary", "multipart/form-data", "bad"},
		{"bad multipart", "multipart/form-data; boundary=example", "bad"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reader := &demandReader{payload: tc.payload, fail: errors.New("must not read")}
			req, _ := http.NewRequest("DELETE", "http://example.invalid/views/1?p=1", reader)
			req.Header.Set("Content-Type", tc.contentType)
			req.Header.Set("X-Test", "header")
			req.AddCookie(&http.Cookie{Name: "test", Value: "cookie"})
			s, err := NewDeferred(req, WithPathParams(map[string]string{"id": "1"}))
			if err != nil {
				t.Fatal(err)
			}
			for _, v := range []struct{ kind, name string }{{QueryKind, "p"}, {PathKind, "id"}, {HeaderKind, "X-Test"}, {CookieKind, "test"}, {HTTPRequestKind, "method"}, {HTTPRequestKind, "uri"}, {HTTPRequestKind, "url"}} {
				if _, ok, err := deferredValue(t, s, v.kind, v.name, nil); err != nil || !ok {
					t.Fatalf("metadata %s: %v %v", v.kind, ok, err)
				}
			}
			for _, p := range s.Providers() {
				p.Kind()
				p.Locate(nil)
			}
			if s.Body() == nil {
				t.Fatal("nil deferred body")
			}
			if err = s.Close(); err != nil {
				t.Fatal(err)
			}
			if reader.reads != 0 {
				t.Fatal("unused body was read")
			}
			if _, _, err := s.Body().Value(context.Background(), nil, ""); err == nil {
				t.Fatal("closed scope initialized body")
			}
			if reader.reads != 0 {
				t.Fatal("closed scope read body")
			}
		})
	}
}

func TestDeferredRequestSnapshotReplayRawAndForms(t *testing.T) {
	for _, rawFirst := range []bool{false, true} {
		t.Run(map[bool]string{false: "body first", true: "raw first"}[rawFirst], func(t *testing.T) {
			payload := `{"id":9007199254740993,"flag":false}`
			reader := &demandReader{payload: payload}
			req, _ := http.NewRequest("DELETE", "http://example.invalid/views/1", reader)
			req.Header.Set("Content-Type", "application/json")
			s, err := NewDeferred(req)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			rawLookup := func() {
				v, ok, err := deferredValue(t, s, HTTPRequestKind, "", nil)
				if err != nil || !ok || v != req {
					t.Fatalf("request identity: %v", err)
				}
				raw, err := io.ReadAll(req.Body)
				if err != nil || string(raw) != payload {
					t.Fatalf("raw snapshot: %v %s", err, raw)
				}
			}
			if rawFirst {
				rawLookup()
			}
			captured, ok, err := s.Body().CaptureSource(context.Background(), reflect.TypeOf(int64(0)), "id")
			if err != nil || !ok || string(captured.(json.RawMessage)) != "9007199254740993" {
				t.Fatalf("precision %v %v", captured, err)
			}
			if !rawFirst {
				rawLookup()
			}
			v, ok, err := s.Body().Value(context.Background(), reflect.TypeOf(false), "flag")
			if err != nil || !ok || v != false {
				t.Fatal("false presence lost")
			}
			if reader.bytes != len(payload) {
				t.Fatalf("underlying bytes read=%d", reader.bytes)
			}
		})
	}
}

func TestDeferredFormBodyOrdersOverridesAndIsolation(t *testing.T) {
	for _, formFirst := range []bool{false, true} {
		t.Run(map[bool]string{false: "body first", true: "form first"}[formFirst], func(t *testing.T) {
			payload := "key=body1&key=body2&empty="
			reader := &demandReader{payload: payload}
			req, _ := http.NewRequest("POST", "http://example.invalid/?key=query", reader)
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			s, err := NewDeferred(req)
			if err != nil {
				t.Fatal(err)
			}
			overlay := s.WithHeader("X-Test", "overlay")
			bodyLookup := func() {
				v, ok, err := s.Body().Value(context.Background(), reflect.TypeOf(""), "")
				if err != nil || !ok || v != payload {
					t.Fatal("body unavailable after form lookup")
				}
			}
			formLookup := func() {
				v, ok, err := deferredValue(t, overlay, FormKind, "key", reflect.TypeOf([]string{}))
				if err != nil || !ok || !reflect.DeepEqual(v, []string{"body1", "body2"}) {
					t.Fatalf("body form values %v %v", v, err)
				}
				v.([]string)[0] = "mutated"
				again, _, _ := deferredValue(t, s, FormKind, "key", reflect.TypeOf([]string{}))
				if !reflect.DeepEqual(again, []string{"body1", "body2"}) {
					t.Fatal("returned form slice aliased")
				}
				if _, ok, err := deferredValue(t, s, FormKind, "empty", nil); err != nil || !ok {
					t.Fatal("explicit empty absent")
				}
			}
			if formFirst {
				formLookup()
				bodyLookup()
			} else {
				bodyLookup()
				formLookup()
			}
			if req.Form != nil || req.PostForm != nil || req.MultipartForm != nil {
				t.Fatal("original request form changed")
			}
			if reader.bytes != len(payload) {
				t.Fatal("body stream reread")
			}
			var wg sync.WaitGroup
			for i := 0; i < 10; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					s.Body().Value(context.Background(), reflect.TypeOf(""), "")
					overlay.Form().Locate(nil).Value(context.Background(), nil, "key")
				}()
			}
			wg.Wait()
			if err := overlay.Close(); err != nil {
				t.Fatal(err)
			}
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
	req, _ := http.NewRequest("POST", "http://example.invalid", &demandReader{fail: errors.New("unused")})
	replacement, _ := NewDeferred(req, WithForm(url.Values{"field": {"override"}}))
	if v, ok, err := deferredValue(t, replacement, FormKind, "field", reflect.TypeOf("")); err != nil || !ok || v != "override" {
		t.Fatal("explicit form override ignored")
	}
	replacement.Close()
}

func TestDeferredRequestReadFailureCached(t *testing.T) {
	cause := errors.New("partial read failure")
	reader := &demandReader{payload: "partial", fail: cause}
	req, _ := http.NewRequest("POST", "http://example.invalid", reader)
	s, err := NewDeferred(req)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for i := 0; i < 3; i++ {
		if _, _, err := s.Body().Value(context.Background(), nil, ""); !errors.Is(err, cause) {
			t.Fatal("body read error lost")
		}
		if _, _, err := deferredValue(t, s, FormKind, "field", nil); !errors.Is(err, cause) {
			t.Fatal("form read error lost")
		}
	}
	if reader.reads != 1 || reader.bytes != 7 {
		t.Fatalf("failure retried %d reads", reader.reads)
	}
}

func TestDeferredMultipartCleanupAndCallerOwnership(t *testing.T) {
	var raw bytes.Buffer
	w := multipart.NewWriter(&raw)
	part, err := w.CreateFormFile("file", "large.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = part.Write(bytes.Repeat([]byte("x"), 33<<20)); err != nil {
		t.Fatal(err)
	}
	w.WriteField("key", "body")
	w.Close()
	req, _ := http.NewRequest("POST", "http://example.invalid/?key=query", bytes.NewReader(raw.Bytes()))
	req.Header.Set("Content-Type", w.FormDataContentType())
	s, err := NewDeferred(req)
	if err != nil {
		t.Fatal(err)
	}
	if v, ok, err := deferredValue(t, s, FormKind, "key", nil); err != nil || !ok || v != "body" {
		t.Fatalf("multipart %v %v", v, err)
	}
	file, err := s.deferred.cleanup.File["file"][0].Open()
	if err != nil {
		t.Fatal(err)
	}
	name := file.(*os.File).Name()
	file.Close()
	if req.MultipartForm != nil {
		t.Fatal("caller multipart modified")
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := s.WithHeader("X-Test", "close").Close(); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if _, err := os.Stat(name); !os.IsNotExist(err) {
		t.Fatalf("owned multipart spill not removed: %v", err)
	}
	caller, _ := http.NewRequest("POST", "http://example.invalid", bytes.NewReader(raw.Bytes()))
	caller.Header.Set("Content-Type", w.FormDataContentType())
	if err := caller.ParseMultipartForm(32 << 20); err != nil {
		t.Fatal(err)
	}
	defer caller.MultipartForm.RemoveAll()
	f, err := caller.MultipartForm.File["file"][0].Open()
	if err != nil {
		t.Fatal(err)
	}
	callerName := f.(*os.File).Name()
	f.Close()
	scope, err := NewDeferred(caller)
	if err != nil {
		t.Fatal(err)
	}
	deferredValue(t, scope, FormKind, "key", nil)
	scope.Close()
	if _, err := os.Stat(callerName); err != nil {
		t.Fatal("removed caller-owned multipart file")
	}
}

func TestDeferredRequestDefaultEagerCompatibility(t *testing.T) {
	req, _ := http.NewRequest("DELETE", "http://example.invalid", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json; broken")
	if _, err := New(req); err == nil {
		t.Fatal("default eager MIME rejection removed")
	}
	req, _ = http.NewRequest("DELETE", "http://example.invalid", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json; broken")
	s, err := NewDeferred(req)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, _, err = s.Body().Value(context.Background(), nil, ""); err == nil {
		t.Fatal("demand suppressed MIME error")
	}
}
