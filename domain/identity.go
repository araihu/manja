package domain

import (
	"fmt"
	"path"
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

func containsControlCharacter(value string) bool {
	for _, character := range value {
		if unicode.IsControl(character) {
			return true
		}
	}
	return false
}
