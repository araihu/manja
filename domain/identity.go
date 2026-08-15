package domain

import (
	"fmt"
	"path"
	"reflect"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ValidateCanonicalIdentity rejects identities that cannot round-trip through
// deterministic UTF-8 persistence or that rely on whitespace/control-byte
// normalization. Provider-neutral application boundaries may use the same
// rule as domain validators before causing external effects.
func ValidateCanonicalIdentity(name, value string, allowEmpty bool) error {
	if !utf8.ValidString(value) {
		return fmt.Errorf("%s must contain valid UTF-8", name)
	}
	if value == "" {
		if allowEmpty {
			return nil
		}
		return fmt.Errorf("%s is required", name)
	}
	if value != strings.TrimSpace(value) {
		return fmt.Errorf("%s must not contain leading or trailing whitespace", name)
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return fmt.Errorf("%s must not contain control characters", name)
		}
	}
	return nil
}

// ValidateCanonicalPublicPath applies the canonical identity rule and the
// provider-neutral absolute, clean path contract used by release/publication
// boundaries.
func ValidateCanonicalPublicPath(name, value string, allowEmpty bool) error {
	if err := ValidateCanonicalIdentity(name, value, allowEmpty); err != nil {
		return err
	}
	if value == "" {
		return nil
	}
	if !strings.HasPrefix(value, "/") || strings.Contains(value, `\`) || path.Clean(value) != value {
		return fmt.Errorf("%s is invalid", name)
	}
	return nil
}

func validateCanonicalIdentity(name, value string, allowEmpty bool) error {
	return ValidateCanonicalIdentity(name, value, allowEmpty)
}

func validateUTF8Strings(name string, value any) error {
	path := utf8Path{root: name}
	return validateUTF8Value(&path, reflect.ValueOf(value), make(map[utf8Visit]struct{}))
}

type utf8Visit struct {
	typ     reflect.Type
	pointer uintptr
}

type utf8PathSegmentKind uint8

const (
	utf8PathField utf8PathSegmentKind = iota
	utf8PathMapKey
	utf8PathMapValue
	utf8PathIndex
)

type utf8PathSegment struct {
	kind  utf8PathSegmentKind
	field string
	index int
}

type utf8Path struct {
	root     string
	segments []utf8PathSegment
}

func (p *utf8Path) pushField(field string) {
	p.segments = append(p.segments, utf8PathSegment{kind: utf8PathField, field: field})
}

func (p *utf8Path) pushMapKey() {
	p.segments = append(p.segments, utf8PathSegment{kind: utf8PathMapKey})
}

func (p *utf8Path) pushMapValue() {
	p.segments = append(p.segments, utf8PathSegment{kind: utf8PathMapValue})
}

func (p *utf8Path) pushIndex(index int) {
	p.segments = append(p.segments, utf8PathSegment{kind: utf8PathIndex, index: index})
}

func (p *utf8Path) pop() {
	p.segments = p.segments[:len(p.segments)-1]
}

func (p *utf8Path) string() string {
	var builder strings.Builder
	builder.Grow(len(p.root) + len(p.segments)*4)
	builder.WriteString(p.root)
	for _, segment := range p.segments {
		switch segment.kind {
		case utf8PathField:
			builder.WriteByte('.')
			builder.WriteString(segment.field)
		case utf8PathMapKey:
			builder.WriteString(" map key")
		case utf8PathMapValue:
			builder.WriteString(" map value")
		case utf8PathIndex:
			builder.WriteByte('[')
			builder.WriteString(strconv.Itoa(segment.index))
			builder.WriteByte(']')
		}
	}
	return builder.String()
}

func (p *utf8Path) invalidUTF8Error() error {
	return fmt.Errorf("%s must contain valid UTF-8", p.string())
}

func validateUTF8Value(path *utf8Path, value reflect.Value, visited map[utf8Visit]struct{}) error {
	if !value.IsValid() {
		return nil
	}
	switch value.Kind() {
	case reflect.Interface:
		if value.IsNil() {
			return nil
		}
		return validateUTF8Value(path, value.Elem(), visited)
	case reflect.Pointer:
		if value.IsNil() {
			return nil
		}
		visit := utf8Visit{typ: value.Type(), pointer: value.Pointer()}
		if _, ok := visited[visit]; ok {
			return nil
		}
		visited[visit] = struct{}{}
		return validateUTF8Value(path, value.Elem(), visited)
	case reflect.String:
		if !utf8.ValidString(value.String()) {
			return path.invalidUTF8Error()
		}
	case reflect.Struct:
		typ := value.Type()
		for index := 0; index < value.NumField(); index++ {
			field := typ.Field(index)
			if field.PkgPath != "" {
				continue
			}
			path.pushField(field.Name)
			err := validateUTF8Value(path, value.Field(index), visited)
			path.pop()
			if err != nil {
				return err
			}
		}
	case reflect.Map:
		iterator := value.MapRange()
		for iterator.Next() {
			path.pushMapKey()
			err := validateUTF8Value(path, iterator.Key(), visited)
			path.pop()
			if err != nil {
				return err
			}
			path.pushMapValue()
			err = validateUTF8Value(path, iterator.Value(), visited)
			path.pop()
			if err != nil {
				return err
			}
		}
	case reflect.Slice, reflect.Array:
		for index := 0; index < value.Len(); index++ {
			path.pushIndex(index)
			err := validateUTF8Value(path, value.Index(index), visited)
			path.pop()
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func containsControlCharacter(value string) bool {
	for _, character := range value {
		if unicode.IsControl(character) {
			return true
		}
	}
	return false
}
