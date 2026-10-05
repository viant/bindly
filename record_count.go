package bindly

import (
	"fmt"
	"reflect"
	"strings"
)

// compileRecordCount validates and detaches authored limits from the immutable plan.
func (b *BindingSpec) compileRecordCount(target reflect.Type) error {
	configured := b.MinAllowedRecords != nil || b.MaxAllowedRecords != nil || b.ExpectedReturned != nil
	if !configured {
		return nil
	}
	if b.countsSourceRecords() {
		target = b.SourceType
	}
	for target.Kind() == reflect.Pointer {
		target = target.Elem()
	}
	if target.Kind() != reflect.Slice && target.Kind() != reflect.Array && target.Kind() != reflect.Struct && !strings.EqualFold(b.Cardinality, "One") {
		return fmt.Errorf("record count constraints require a collection or a singleton record")
	}
	for _, limit := range []**int{&b.MinAllowedRecords, &b.MaxAllowedRecords, &b.ExpectedReturned} {
		if *limit == nil {
			continue
		}
		value := **limit
		if value < 0 {
			return fmt.Errorf("record count constraints must be nonnegative")
		}
		*limit = &value
	}
	if b.MinAllowedRecords != nil && b.MaxAllowedRecords != nil && *b.MinAllowedRecords > *b.MaxAllowedRecords {
		return fmt.Errorf("minimum record count exceeds maximum")
	}
	if b.ExpectedReturned != nil {
		if b.MinAllowedRecords != nil && *b.ExpectedReturned < *b.MinAllowedRecords || b.MaxAllowedRecords != nil && *b.ExpectedReturned > *b.MaxAllowedRecords {
			return fmt.Errorf("expected record count is outside allowed range")
		}
	}
	return nil
}

// A collection/record codec input is counted before a codec can collapse it.
// Scalar encodings (CSV or JSON text) are counted after decoding instead.
func (b *BindingSpec) countsSourceRecords() bool {
	t := b.SourceType
	for t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t != nil && (t.Kind() == reflect.Slice || t.Kind() == reflect.Array || t.Kind() == reflect.Struct)
}

func (b *BindingSpec) validateRecordCount(value any) error {
	v := reflect.ValueOf(value)
	for v.IsValid() && v.Kind() == reflect.Pointer {
		if v.IsNil() {
			v = reflect.Value{}
		} else {
			v = v.Elem()
		}
	}
	count := 0
	if v.IsValid() {
		switch v.Kind() {
		case reflect.Slice, reflect.Array:
			count = v.Len()
		case reflect.Struct:
			count = 1
		default:
			if !strings.EqualFold(b.Cardinality, "One") {
				return nil
			}
			count = 1
		}
	}
	switch {
	case b.MinAllowedRecords != nil && count < *b.MinAllowedRecords:
		return fmt.Errorf("expected at least %d records, but had %d", *b.MinAllowedRecords, count)
	case b.ExpectedReturned != nil && count != *b.ExpectedReturned:
		return fmt.Errorf("expected %d records, but had %d", *b.ExpectedReturned, count)
	case b.MaxAllowedRecords != nil && count > *b.MaxAllowedRecords:
		return fmt.Errorf("expected no more than %d records, but had %d", *b.MaxAllowedRecords, count)
	case count == 0 && b.Required != nil && *b.Required:
		return fmt.Errorf("parameter %s value is required but no data was found", b.Name)
	}
	return nil
}
