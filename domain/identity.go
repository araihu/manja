package domain

import (
	"fmt"
	"path"
	"reflect"
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
	return validateUTF8Value(name, reflect.ValueOf(value), make(map[utf8Visit]struct{}))
}

type utf8Visit struct {
	typ     reflect.Type
	pointer uintptr
}

func validateUTF8Value(name string, value reflect.Value, visited map[utf8Visit]struct{}) error {
	if !value.IsValid() {
		return nil
	}
	switch value.Kind() {
	case reflect.Interface:
		if value.IsNil() {
			return nil
		}
		return validateUTF8Value(name, value.Elem(), visited)
	case reflect.Pointer:
		if value.IsNil() {
			return nil
		}
		visit := utf8Visit{typ: value.Type(), pointer: value.Pointer()}
		if _, ok := visited[visit]; ok {
			return nil
		}
		visited[visit] = struct{}{}
		return validateUTF8Value(name, value.Elem(), visited)
	case reflect.String:
		if !utf8.ValidString(value.String()) {
			return fmt.Errorf("%s must contain valid UTF-8", name)
		}
	case reflect.Struct:
		typ := value.Type()
		for index := 0; index < value.NumField(); index++ {
			field := typ.Field(index)
			if field.PkgPath != "" {
				continue
			}
			if err := validateUTF8Value(name+"."+field.Name, value.Field(index), visited); err != nil {
				return err
			}
		}
	case reflect.Map:
		iterator := value.MapRange()
		for iterator.Next() {
			if err := validateUTF8Value(name+" map key", iterator.Key(), visited); err != nil {
				return err
			}
			if err := validateUTF8Value(name+" map value", iterator.Value(), visited); err != nil {
				return err
			}
		}
	case reflect.Slice, reflect.Array:
		for index := 0; index < value.Len(); index++ {
			if err := validateUTF8Value(fmt.Sprintf("%s[%d]", name, index), value.Index(index), visited); err != nil {
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
