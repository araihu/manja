package projection

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"net/url"
	"path"
	"strings"
	"unicode"
)

func operationAnchor(operationID, supplied, method, operationPath string) (string, error) {
	if supplied != "" {
		if supplied != strings.TrimSpace(supplied) || !validSuppliedAnchor(supplied) {
			return "", projectionFailure("operation.anchor", "invalid_source")
		}
		return supplied, nil
	}
	value := slugASCII(operationID)
	if value == "" {
		value = slugASCII(strings.ToUpper(method) + " " + operationPath)
	}
	if value == "" {
		return "", projectionFailure("operation.anchor", "invalid_source")
	}
	return "operation-" + value, nil
}

func validSuppliedAnchor(value string) bool {
	for index, r := range value {
		if r > unicode.MaxASCII {
			return false
		}
		if index == 0 {
			if !asciiLowerOrDigit(byte(r)) {
				return false
			}
			continue
		}
		if !asciiLowerOrDigit(byte(r)) && !strings.ContainsRune("._~/-", r) {
			return false
		}
	}
	return value != ""
}

func asciiLowerOrDigit(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= '0' && value <= '9'
}

func slugASCII(value string) string {
	var result strings.Builder
	dash := false
	for _, r := range value {
		switch {
		case r >= 'A' && r <= 'Z':
			result.WriteByte(byte(r + ('a' - 'A')))
			dash = false
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			result.WriteByte(byte(r))
			dash = false
		default:
			if result.Len() != 0 {
				dash = true
			}
		}
		if dash {
			current := result.String()
			if !strings.HasSuffix(current, "-") {
				result.WriteByte('-')
			}
			dash = false
		}
	}
	return strings.Trim(result.String(), "-")
}

func selectedHref(anchor string) string {
	return "?selected=" + url.QueryEscape(anchor) + "#" + anchor
}

func recordID(kind string, parts ...string) string {
	hash := sha256.New()
	hash.Write([]byte(kind))
	hash.Write([]byte{0})
	var length [8]byte
	for _, part := range parts {
		binary.BigEndian.PutUint64(length[:], uint64(len([]byte(part))))
		hash.Write(length[:])
		hash.Write([]byte(part))
	}
	return kind + "-" + hex.EncodeToString(hash.Sum(nil))
}

func parseSelectedReference(raw string) (string, error) {
	reference, err := url.Parse(raw)
	if err != nil || reference.IsAbs() || reference.Host != "" || reference.User != nil || reference.Path != "" || reference.Fragment == "" || strings.Contains(raw, "\\") {
		return "", projectionFailure("navigation.href", "invalid_source")
	}
	query := reference.Query()
	if len(query) > 1 {
		return "", projectionFailure("navigation.href", "invalid_source")
	}
	if len(query) == 1 {
		values, ok := query["selected"]
		if !ok || len(values) != 1 || values[0] != reference.Fragment {
			return "", projectionFailure("navigation.href", "invalid_source")
		}
	}
	return reference.Fragment, nil
}

func canonicalPublicRoute(raw string, resolve func(string) (string, bool)) (string, error) {
	reference, err := url.Parse(raw)
	if err != nil || reference.IsAbs() || reference.Host != "" || reference.User != nil || strings.Contains(raw, "\\") || strings.TrimSpace(raw) != raw {
		return "", projectionFailure("publicRoutes.path", "invalid_source")
	}
	if raw == "/" {
		return raw, nil
	}
	if reference.Path == "" || !strings.HasPrefix(reference.Path, "/") || path.Clean(reference.Path) != reference.Path || reference.Fragment == "" {
		return "", projectionFailure("publicRoutes.path", "invalid_source")
	}
	query := reference.Query()
	values, ok := query["selected"]
	if !ok || len(query) != 1 || len(values) != 1 || values[0] != reference.Fragment {
		return "", projectionFailure("publicRoutes.path", "invalid_source")
	}
	anchor, ok := resolve(reference.Fragment)
	if !ok {
		return "", projectionFailure("publicRoutes.path", "invalid_source")
	}
	return reference.Path + selectedHref(anchor), nil
}
