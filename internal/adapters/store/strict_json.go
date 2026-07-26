package store

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"unicode/utf8"
)

// strictJSONUnmarshal rejects Unicode surrogate escapes that encoding/json
// would otherwise replace with U+FFFD and duplicate decoded object member
// names that encoding/json would silently overwrite. Persisted evidence must
// never be normalized or selected by input order while decoding.
func strictJSONUnmarshal(data []byte, value any) error {
	if !utf8.Valid(data) {
		return fmt.Errorf("persisted JSON must contain valid UTF-8")
	}
	if !json.Valid(data) {
		return json.Unmarshal(data, value)
	}
	if err := validateJSONSurrogateEscapes(data); err != nil {
		return err
	}
	if err := validateJSONMemberNames(data); err != nil {
		return err
	}
	return json.Unmarshal(data, value)
}

func validateJSONMemberNames(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := validateJSONValueMemberNames(decoder); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		if err == nil {
			return fmt.Errorf("persisted JSON contains multiple top-level values")
		}
		return fmt.Errorf("read persisted JSON after top-level value: %w", err)
	}
	return nil
}

func validateJSONValueMemberNames(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return fmt.Errorf("read persisted JSON value: %w", err)
	}
	delimiter, isDelimiter := token.(json.Delim)
	if !isDelimiter {
		return nil
	}
	switch delimiter {
	case '{':
		seen := make(map[string]struct{})
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return fmt.Errorf("read persisted JSON object member: %w", err)
			}
			key, ok := keyToken.(string)
			if !ok {
				return fmt.Errorf("persisted JSON object member name is not a string")
			}
			if _, duplicate := seen[key]; duplicate {
				return fmt.Errorf("persisted JSON object contains duplicate decoded member name %q", key)
			}
			seen[key] = struct{}{}
			if err := validateJSONValueMemberNames(decoder); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil {
			return fmt.Errorf("close persisted JSON object: %w", err)
		}
		if end != json.Delim('}') {
			return fmt.Errorf("persisted JSON object has invalid closing delimiter")
		}
	case '[':
		for decoder.More() {
			if err := validateJSONValueMemberNames(decoder); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil {
			return fmt.Errorf("close persisted JSON array: %w", err)
		}
		if end != json.Delim(']') {
			return fmt.Errorf("persisted JSON array has invalid closing delimiter")
		}
	default:
		return fmt.Errorf("persisted JSON contains unexpected delimiter %q", delimiter)
	}
	return nil
}

func validateJSONSurrogateEscapes(data []byte) error {
	for index := 0; index < len(data); index++ {
		if data[index] != '"' {
			continue
		}
		for index++; index < len(data) && data[index] != '"'; index++ {
			if data[index] != '\\' {
				continue
			}
			index++
			if index >= len(data) || data[index] != 'u' {
				continue
			}
			unit, ok := jsonHexQuad(data, index+1)
			if !ok {
				return fmt.Errorf("persisted JSON contains an invalid Unicode escape")
			}
			index += 4
			switch {
			case unit >= 0xd800 && unit <= 0xdbff:
				if index+6 >= len(data) || data[index+1] != '\\' || data[index+2] != 'u' {
					return fmt.Errorf("persisted JSON contains an unpaired high surrogate escape")
				}
				low, ok := jsonHexQuad(data, index+3)
				if !ok || low < 0xdc00 || low > 0xdfff {
					return fmt.Errorf("persisted JSON contains an invalid surrogate pair")
				}
				index += 6
			case unit >= 0xdc00 && unit <= 0xdfff:
				return fmt.Errorf("persisted JSON contains an unpaired low surrogate escape")
			}
		}
	}
	return nil
}

func jsonHexQuad(data []byte, start int) (uint16, bool) {
	if start < 0 || start+4 > len(data) {
		return 0, false
	}
	var value uint16
	for _, digit := range data[start : start+4] {
		value <<= 4
		switch {
		case digit >= '0' && digit <= '9':
			value |= uint16(digit - '0')
		case digit >= 'a' && digit <= 'f':
			value |= uint16(digit-'a') + 10
		case digit >= 'A' && digit <= 'F':
			value |= uint16(digit-'A') + 10
		default:
			return 0, false
		}
	}
	return value, true
}
