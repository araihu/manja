package domain

import (
	"fmt"
	"net/url"
	"path"
	"strings"
)

// EffectiveOperationRequestTarget returns the literal request target shown to
// users and used by generated request snippets.
func EffectiveOperationRequestTarget(operation Operation) string {
	if operation.RequestTarget != "" {
		return operation.RequestTarget
	}
	return operation.Path
}

// ValidateOperationRequestTarget checks the canonical path and the optional
// vendor fixed-query representation without normalizing the source literal.
func ValidateOperationRequestTarget(operationPath, requestTarget string, fixed []FixedQueryParameter) error {
	if err := validateOperationPath(operationPath); err != nil {
		return err
	}
	if requestTarget == "" {
		if len(fixed) != 0 {
			return fmt.Errorf("operation fixed query requires a request target")
		}
		return nil
	}
	if err := ValidateCanonicalIdentity("operation request target", requestTarget, false); err != nil {
		return err
	}
	prefix := operationPath + "?"
	if !strings.HasPrefix(requestTarget, prefix) || strings.ContainsAny(requestTarget, " #") {
		return fmt.Errorf("operation request target is invalid")
	}
	rawQuery := strings.TrimPrefix(requestTarget, prefix)
	if rawQuery == "" {
		return fmt.Errorf("operation request target query is required")
	}
	parts := strings.Split(rawQuery, "&")
	if len(parts) != len(fixed) {
		return fmt.Errorf("operation fixed query differs from request target")
	}
	for index, part := range parts {
		if part == "" {
			return fmt.Errorf("operation request target query item %d is empty", index)
		}
		rawName, rawValue, hasValue := strings.Cut(part, "=")
		if !hasValue {
			rawValue = ""
		}
		name, err := url.QueryUnescape(rawName)
		if err != nil || name == "" {
			return fmt.Errorf("operation request target query name %d is invalid", index)
		}
		value, err := url.QueryUnescape(rawValue)
		if err != nil {
			return fmt.Errorf("operation request target query value %d is invalid", index)
		}
		if err := ValidateCanonicalIdentity(fmt.Sprintf("operation fixed query %d name", index), fixed[index].Name, false); err != nil {
			return err
		}
		if err := ValidateCanonicalIdentity(fmt.Sprintf("operation fixed query %d value", index), fixed[index].Value, true); err != nil {
			return err
		}
		if fixed[index].Name != name || fixed[index].Value != value {
			return fmt.Errorf("operation fixed query item %d differs from request target", index)
		}
	}
	return nil
}

func validateOperationPath(value string) error {
	if err := ValidateCanonicalIdentity("operation path", value, false); err != nil {
		return err
	}
	cleanInput := value
	if value != "/" && strings.HasSuffix(value, "/") {
		cleanInput = strings.TrimSuffix(value, "/")
	}
	if !strings.HasPrefix(value, "/") || path.Clean(cleanInput) != cleanInput || strings.ContainsAny(value, " ?#") {
		return fmt.Errorf("operation path is invalid")
	}
	return nil
}
