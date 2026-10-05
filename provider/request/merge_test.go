package request

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"testing"
)

func TestMultipartThresholdAndOverlayShareOwnedCleanup(t *testing.T) {
	var encoded bytes.Buffer
	writer := multipart.NewWriter(&encoded)
	if err := writer.WriteField("name", "original"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"first.txt"} {
		part, err := writer.CreateFormFile("upload", name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = io.WriteString(part, "content"); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request, _ := http.NewRequest("POST", "http://example.com", bytes.NewReader(encoded.Bytes()))
	request.Header.Set("Content-Type", writer.FormDataContentType())
	scope, err := New(request, WithMaxMultipartMemory(1), WithForm(url.Values{"name": {"overlay"}}))
	if err != nil {
		t.Fatal(err)
	}
	defer scope.Close()
	if request.MultipartForm != nil {
		t.Fatal("request multipart ownership changed")
	}
	files, found, err := scope.Body().Value(context.Background(), reflect.TypeOf([]*multipart.FileHeader{}), "upload")
	if err != nil || !found || len(files.([]*multipart.FileHeader)) != 1 {
		t.Fatalf("files %v found %v error %v", files, found, err)
	}
	file, err := files.([]*multipart.FileHeader)[0].Open()
	if err != nil {
		t.Fatal(err)
	}
	actual, ok := file.(*os.File)
	if !ok {
		t.Fatalf("configured threshold did not spill: %T", file)
	}
	path := actual.Name()
	file.Close()
	value, found, err := scope.Form().Locate(nil).Value(context.Background(), reflect.TypeOf(""), "name")
	if err != nil || !found || value != "overlay" {
		t.Fatalf("form overlay %v %v %v", value, found, err)
	}
	overlay := scope.WithHeader("Authorization", "token")
	if err = overlay.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("owned file remains: %v", err)
	}
	if err = scope.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestRequestMetadataAndUnnamedSources(t *testing.T) {
	request, _ := http.NewRequest("GET", "http://example.com/users?z=%2f&a=1", nil)
	request.RequestURI = "/users?z=%2f&a=1"
	request.RemoteAddr = "127.0.0.1:1234"
	scope, err := New(request)
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name string
		want any
	}{
		{"", request}, {"METHOD", request.Method}, {"uri", request.RequestURI}, {"host", request.Host}, {"proto", request.Proto}, {"remoteaddr", request.RemoteAddr}, {"header", request.Header}, {"url", request.URL},
	} {
		value, found, err := scope.Request().Locate(nil).Value(context.Background(), reflect.TypeOf(tt.want), tt.name)
		if err != nil || !found || !reflect.DeepEqual(value, tt.want) {
			t.Fatalf("metadata %s: %v %v %v", tt.name, value, found, err)
		}
	}
	query, found, err := scope.Query().Locate(nil).Value(context.Background(), reflect.TypeOf(""), "")
	if err != nil || !found || query != request.URL.RawQuery {
		t.Fatalf("query %v %v %v", query, found, err)
	}
	path, found, err := scope.Path().Locate(nil).Value(context.Background(), reflect.TypeOf(""), "")
	if err != nil || !found || path != request.URL.Path {
		t.Fatalf("path %v %v %v", path, found, err)
	}
}
