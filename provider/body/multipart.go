package body

import (
	"mime/multipart"
	"reflect"
)

// WithMultipartForm supplies a transport-owned parsed form. The transport retains
// responsibility for removing any temporary files after the invocation.
func WithMultipartForm(form *multipart.Form) Option { return func(s *Source) { s.multipart = form } }
func MultipartValue(form *multipart.Form, target reflect.Type, name string) (any, bool) {
	if form == nil || name == "" {
		return nil, false
	}
	fileType := reflect.TypeOf((*multipart.FileHeader)(nil))
	if target == fileType {
		files := form.File[name]
		if len(files) == 0 {
			return nil, false
		}
		return files[0], true
	}
	if target != nil && target.Kind() == reflect.Slice && target.Elem() == fileType {
		files := form.File[name]
		return files, len(files) > 0
	}
	values, found := form.Value[name]
	if !found || len(values) == 0 {
		return nil, false
	}
	for target != nil && target.Kind() == reflect.Pointer {
		target = target.Elem()
	}
	if target != nil && target.Kind() != reflect.Slice && target.Kind() != reflect.Array {
		return values[0], true
	}
	if target == nil && len(values) == 1 {
		return values[0], true
	}
	return values, true
}
