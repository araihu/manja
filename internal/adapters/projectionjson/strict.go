package projectionjson

import (
	"bytes"
	"encoding/json"
	"io"
	"math"
	"regexp"
	"strconv"
	"strings"
)

var (
	canonicalWireNumberPattern     = regexp.MustCompile(`^(0|[1-9][0-9]*)$`)
	canonicalEmbeddedNumberPattern = regexp.MustCompile(`^-?(0|[1-9][0-9]*)(\.[0-9]+)?$`)
)

func validateJSONTokens(input []byte, maxDepth int, validateNumber func(string) bool) error {
	decoder := json.NewDecoder(bytes.NewReader(input))
	decoder.UseNumber()
	if err := validateJSONValue(decoder, 0, maxDepth, validateNumber); err != nil {
		return codecFailure("document", "duplicate_key")
	}
	if _, err := decoder.Token(); err != io.EOF {
		return codecFailure("document", "non_canonical")
	}
	return nil
}

func validateJSONValue(decoder *json.Decoder, depth, maxDepth int, validateNumber func(string) bool) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	switch token := token.(type) {
	case json.Delim:
		if depth >= maxDepth {
			return io.ErrUnexpectedEOF
		}
		switch token {
		case '{':
			seen := make(map[string]struct{})
			for decoder.More() {
				keyToken, err := decoder.Token()
				if err != nil {
					return err
				}
				key, ok := keyToken.(string)
				if !ok {
					return io.ErrUnexpectedEOF
				}
				if _, duplicate := seen[key]; duplicate {
					return io.ErrUnexpectedEOF
				}
				seen[key] = struct{}{}
				if err := validateJSONValue(decoder, depth+1, maxDepth, validateNumber); err != nil {
					return err
				}
			}
			closing, err := decoder.Token()
			if err != nil || closing != json.Delim('}') {
				return io.ErrUnexpectedEOF
			}
		case '[':
			for decoder.More() {
				if err := validateJSONValue(decoder, depth+1, maxDepth, validateNumber); err != nil {
					return err
				}
			}
			closing, err := decoder.Token()
			if err != nil || closing != json.Delim(']') {
				return io.ErrUnexpectedEOF
			}
		default:
			return io.ErrUnexpectedEOF
		}
	case json.Number:
		if !validateNumber(string(token)) {
			return strconv.ErrSyntax
		}
	case string, bool, nil:
		return nil
	default:
		return io.ErrUnexpectedEOF
	}
	return nil
}

func canonicalWireNumber(input string) bool {
	if !canonicalWireNumberPattern.MatchString(input) {
		return false
	}
	value, err := strconv.ParseUint(input, 10, 32)
	return err == nil && value <= math.MaxUint32
}

func canonicalEmbeddedNumber(input string) bool {
	if !canonicalEmbeddedNumberPattern.MatchString(input) || input == "-0" {
		return false
	}
	if dot := strings.IndexByte(input, '.'); dot >= 0 && strings.HasSuffix(input, "0") {
		return false
	}
	return true
}
