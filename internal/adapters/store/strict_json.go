package store

import (
	"encoding/json"
	"fmt"
	"unicode/utf8"
)

// strictJSONUnmarshal rejects Unicode surrogate escapes that encoding/json
// would otherwise replace with U+FFFD. Persisted identity bytes must never be
// normalized while decoding.
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
	return json.Unmarshal(data, value)
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
