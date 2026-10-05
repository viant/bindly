package bindly

import (
	"context"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/viant/bindly/state"
	"github.com/viant/tagly/tags"
)

const (
	bindingTag        = "bind"
	parameterTagAlias = "parameter"
)

// extractBinding parses the complete Datly parameter surface from the generic
// bind tag. Runtime provider resolution remains independent of tag parsing.
func (b *Injector) extractBinding(aBinding *Binding, tagName string) error {
	return parseBindingTag(aBinding, aBinding.selector.Tag(), aBinding.selector.Path(), tagName)
}

func resolveBindingTag(structTag reflect.StructTag, primary string) (string, bool, error) {
	names := []string{primary}
	if primary == bindingTag {
		names = append(names, parameterTagAlias)
	}
	matched := ""
	for _, name := range names {
		if _, ok := structTag.Lookup(name); !ok {
			continue
		}
		if matched != "" {
			return "", false, fmt.Errorf("binding tags %q and %q cannot both be declared", matched, name)
		}
		matched = name
	}
	return matched, matched != "", nil
}

func parseBindingTag(aBinding *Binding, structTag reflect.StructTag, defaultName, tagName string) error {
	tag, ok := structTag.Lookup(tagName)
	if !ok {
		return nil
	}
	name, values := tags.Values(tag).Name()
	aBinding.Name = strings.TrimSpace(name)
	aBinding.Tags = values
	if value, ok := structTag.Lookup("value"); ok {
		aBinding.DefaultValue = value
	}
	if err := values.MatchPairs(func(key, value string) error {
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "in":
			aBinding.location.In = strings.Trim(strings.TrimSpace(value), "{}")
		case "kind":
			aBinding.location.Kind = strings.ToLower(strings.TrimSpace(value))
		case "cacheable":
			parsed, err := parseBindBool(value)
			if err != nil {
				return fmt.Errorf("invalid cacheable value %q: %w", value, err)
			}
			aBinding.Cacheable = &parsed
		case "required":
			parsed, err := parseBindBool(value)
			if err != nil {
				return fmt.Errorf("invalid required value %q: %w", value, err)
			}
			aBinding.Required = &parsed
		case "scope":
			aBinding.Scope = strings.TrimSpace(value)
		case "name":
			aBinding.Name = strings.TrimSpace(value)
		case "when":
			aBinding.When = strings.TrimSpace(value)
		case "errorcode":
			parsed, err := strconv.Atoi(strings.TrimSpace(value))
			if err != nil {
				return fmt.Errorf("invalid errorCode value %q: %w", value, err)
			}
			aBinding.ErrorCode = parsed
		case "errormessage":
			aBinding.ErrorMessage = strings.TrimSpace(value)
		case "datatype":
			aBinding.DataType = strings.TrimSpace(value)
		case "cardinality":
			aBinding.Cardinality = strings.TrimSpace(value)
		case "with":
			aBinding.With = strings.TrimSpace(value)
		case "async":
			parsed, err := parseBindBool(value)
			if err != nil {
				return fmt.Errorf("invalid async value %q: %w", value, err)
			}
			aBinding.Async = parsed
		case "uri":
			aBinding.URI = strings.TrimSpace(value)
		case "resource":
			aBinding.ResourceRef = strings.TrimSpace(value)
		case "value":
			aBinding.DefaultValue = strings.Trim(value, "' ")
		default:
			return fmt.Errorf("unsupported bind tag key %q", key)
		}
		return nil
	}); err != nil {
		return err
	}

	if aBinding.location.Kind == "" && aBinding.location.In != "" {
		aBinding.location.Kind = "state"
	}
	// default name from selector path when not explicitly set
	if aBinding.Name == "" {
		aBinding.Name = defaultName
	}
	return nil
}

// BindingSpecFromField parses one Go field's bind or parameter tag into
// explicit plan metadata. Bind is canonical; parameter is the supported Datly
// shape alias.
func BindingSpecFromField(field reflect.StructField) (BindingSpec, bool, error) {
	tagName, ok, err := resolveBindingTag(field.Tag, bindingTag)
	if err != nil {
		return BindingSpec{}, false, err
	}
	if !ok {
		return BindingSpec{}, false, nil
	}
	binding := &Binding{
		path:      field.Name,
		fieldType: field.Type,
		location:  &state.Location{},
		Tag:       field.Tag,
	}
	if err := parseBindingTag(binding, field.Tag, field.Name, tagName); err != nil {
		return BindingSpec{}, false, err
	}
	return BindingSpec{
		Path:         field.Name,
		Location:     *binding.location,
		Name:         binding.Name,
		Scope:        binding.Scope,
		When:         binding.When,
		ErrorCode:    binding.ErrorCode,
		ErrorMessage: binding.ErrorMessage,
		DataType:     binding.DataType,
		Cardinality:  binding.Cardinality,
		With:         binding.With,
		URI:          binding.URI,
		ResourceRef:  binding.ResourceRef,
		Required:     cloneBool(binding.Required),
		Cacheable:    cloneBool(binding.Cacheable),
		Async:        binding.Async,
		DefaultValue: binding.DefaultValue,
	}, true, nil
}

func parseBindBool(value string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "true", "1":
		return true, nil
	case "false", "0":
		return false, nil
	default:
		return false, fmt.Errorf("expected true, false, 1, or 0")
	}
}

const xFormTag = "xform"

// extractTransformer extracts transformer from struct tag
func (b *Injector) extractTransformer(ctx context.Context, aBinding *Binding) error {
	// Check if field has a transformer configured
	name := ""
	tag, ok := aBinding.selector.Tag().Lookup(b.xformTag)
	if !ok {
		return nil
	}
	tagValue := tags.Values(tag)
	name, tagValue = tagValue.Name()
	factory, ok := b.transformers.Lookup(name)
	if !ok {
		return fmt.Errorf("failed to lookup transformer: %v", name)
	}
	transformer, err := factory.Create(ctx, tagValue, aBinding.selector.Type(), b.resources)
	if err != nil {
		return fmt.Errorf("failed to create transformer: %v, %w", name, err)
	}
	aBinding.transformer = transformer
	return nil
}
