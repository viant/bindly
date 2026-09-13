package bindly

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/viant/bindly/state"
	"github.com/viant/bindly/xform"
	"github.com/viant/tagly/tags"
)

type BindingSpec struct {
	MarkerField                                                              string
	Path, Name                                                               string
	Location                                                                 state.Location
	SourceType                                                               reflect.Type
	Required, Cacheable                                                      *bool
	MinAllowedRecords, MaxAllowedRecords, ExpectedReturned                   *int
	When, Scope, With, URI, ResourceRef, DataType, ErrorMessage, Cardinality string
	Async                                                                    bool
	DefaultValue, Extension                                                  any
	ErrorCode                                                                int
	Transformer                                                              xform.Transformer
}

func BindingSpecFromField(field reflect.StructField) (BindingSpec, bool, error) {
	return bindingSpecFromField(field, "bind", true)
}

func bindingSpecFromField(field reflect.StructField, key string, aliases bool) (BindingSpec, bool, error) {
	source, found := field.Tag.Lookup(key)
	if aliases {
		if alternate, ok := field.Tag.Lookup("parameter"); ok {
			if found {
				return BindingSpec{}, false, fmt.Errorf("field %s declares both bind and parameter", field.Name)
			}
			source, found = alternate, true
		}
	}
	if !found {
		return BindingSpec{}, false, nil
	}
	result := BindingSpec{Path: field.Name, Name: field.Name}
	name, options := tags.Values(source).Name()
	if name != "" {
		result.Name = name
	}
	// A first key=value pair is not a positional binding name.
	if strings.Contains(name, "=") {
		options = tags.Values(source)
		result.Name = field.Name
	}
	err := options.MatchPairs(func(key, value string) error {
		var err error
		value, err = decodeBindingScalar(value)
		if err != nil {
			return err
		}
		switch key {
		case "kind":
			result.Location.Kind = value
		case "in":
			result.Location.In = value
		case "name":
			result.Name = value
		case "required", "cacheable":
			if value == "" {
				value = "true"
			}
			v, err := strconv.ParseBool(value)
			if err != nil {
				return err
			}
			if key == "required" {
				result.Required = &v
			} else {
				result.Cacheable = &v
			}
		case "when":
			result.When = value
		case "scope":
			result.Scope = value
		case "with":
			result.With = value
		case "uri":
			result.URI = value
		case "resource", "resourceRef":
			result.ResourceRef = value
		case "async":
			if value == "" {
				value = "true"
			}
			v, err := strconv.ParseBool(value)
			if err != nil {
				return err
			}
			result.Async = v
		case "value", "default":
			result.DefaultValue = value
		case "dataType", "type":
			result.DataType = value
		case "errorCode", "errorStatusCode":
			v, err := strconv.Atoi(value)
			if err != nil {
				return err
			}
			result.ErrorCode = v
		case "errorMessage":
			result.ErrorMessage = value
		case "minAllowedRecords", "maxAllowedRecords", "expectedReturned":
			v, err := strconv.Atoi(value)
			if err != nil {
				return err
			}
			switch key {
			case "minAllowedRecords":
				result.MinAllowedRecords = &v
			case "maxAllowedRecords":
				result.MaxAllowedRecords = &v
			case "expectedReturned":
				result.ExpectedReturned = &v
			}
		case "cardinality":
			result.Cardinality = value
		}
		return nil
	})
	if result.Location.Kind == "" && result.Location.In != "" {
		result.Location.Kind = "state"
	}
	return result, true, err
}

// Tagly tokenizes comma-bearing scalar quotes without splitting their content.
// Its generic unquote step leaves multi-character single-quoted tokens intact;
// binding metadata decodes that supported scalar form using Go's escape parser.
func decodeBindingScalar(value string) (string, error) {
	if len(value) == 0 || value[0] != '\'' {
		return value, nil
	}
	if len(value) < 2 || value[len(value)-1] != '\'' {
		return "", fmt.Errorf("unterminated binding scalar")
	}
	remaining := value[1 : len(value)-1]
	var result strings.Builder
	for remaining != "" {
		character, _, tail, err := strconv.UnquoteChar(remaining, '\'')
		if err != nil {
			return "", err
		}
		result.WriteRune(character)
		remaining = tail
	}
	return result.String(), nil
}
