package projection

import (
	"bytes"
	"encoding/json"
	"io"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

const maxEmbeddedJSONBytes = 512 * 1024

type embeddedValue struct {
	kind   byte
	text   string
	array  []embeddedValue
	object map[string]embeddedValue
}

func canonicalEmbeddedJSON(input string) (string, error) {
	return canonicalEmbeddedJSONWithResourceLimits(input, true)
}

func canonicalEmbeddedJSONWithResourceLimits(input string, resourceLimits bool) (string, error) {
	if !utf8.ValidString(input) || resourceLimits && len(input) > maxEmbeddedJSONBytes {
		return "", projectionFailure("embeddedJSON", "invalid_utf8")
	}
	decoder := json.NewDecoder(strings.NewReader(input))
	decoder.UseNumber()
	value, err := decodeEmbeddedValue(decoder, 0, resourceLimits)
	if err != nil {
		return "", projectionFailure("embeddedJSON", "invalid_source")
	}
	if _, err := decoder.Token(); err != io.EOF {
		return "", projectionFailure("embeddedJSON", "invalid_source")
	}
	var output bytes.Buffer
	if err := emitEmbeddedValue(&output, value, resourceLimits); err != nil || resourceLimits && output.Len() > maxEmbeddedJSONBytes {
		return "", projectionFailure("embeddedJSON", "invalid_source")
	}
	return output.String(), nil
}

func decodeEmbeddedValue(decoder *json.Decoder, depth int, resourceLimits bool) (embeddedValue, error) {
	token, err := decoder.Token()
	if err != nil {
		return embeddedValue{}, err
	}
	switch token := token.(type) {
	case json.Delim:
		if depth >= 64 {
			return embeddedValue{}, io.ErrUnexpectedEOF
		}
		switch token {
		case '{':
			result := embeddedValue{kind: 'o', object: make(map[string]embeddedValue)}
			for decoder.More() {
				keyToken, err := decoder.Token()
				if err != nil {
					return embeddedValue{}, err
				}
				key, ok := keyToken.(string)
				if !ok {
					return embeddedValue{}, io.ErrUnexpectedEOF
				}
				if _, exists := result.object[key]; exists {
					return embeddedValue{}, io.ErrUnexpectedEOF
				}
				value, err := decodeEmbeddedValue(decoder, depth+1, resourceLimits)
				if err != nil {
					return embeddedValue{}, err
				}
				result.object[key] = value
			}
			closing, err := decoder.Token()
			if err != nil || closing != json.Delim('}') {
				return embeddedValue{}, io.ErrUnexpectedEOF
			}
			return result, nil
		case '[':
			result := embeddedValue{kind: 'a', array: []embeddedValue{}}
			for decoder.More() {
				value, err := decodeEmbeddedValue(decoder, depth+1, resourceLimits)
				if err != nil {
					return embeddedValue{}, err
				}
				result.array = append(result.array, value)
			}
			closing, err := decoder.Token()
			if err != nil || closing != json.Delim(']') {
				return embeddedValue{}, io.ErrUnexpectedEOF
			}
			return result, nil
		default:
			return embeddedValue{}, io.ErrUnexpectedEOF
		}
	case string:
		return embeddedValue{kind: 's', text: token}, nil
	case json.Number:
		number, err := normalizeJSONNumber(string(token), resourceLimits)
		if err != nil {
			return embeddedValue{}, err
		}
		return embeddedValue{kind: 'n', text: number}, nil
	case bool:
		if token {
			return embeddedValue{kind: 'l', text: "true"}, nil
		}
		return embeddedValue{kind: 'l', text: "false"}, nil
	case nil:
		return embeddedValue{kind: 'l', text: "null"}, nil
	default:
		return embeddedValue{}, io.ErrUnexpectedEOF
	}
}

func emitEmbeddedValue(output *bytes.Buffer, value embeddedValue, resourceLimits bool) error {
	switch value.kind {
	case 's':
		encoded, err := json.Marshal(value.text)
		if err != nil {
			return err
		}
		output.Write(encoded)
	case 'n', 'l':
		output.WriteString(value.text)
	case 'a':
		output.WriteByte('[')
		for index, item := range value.array {
			if index != 0 {
				output.WriteByte(',')
			}
			if err := emitEmbeddedValue(output, item, resourceLimits); err != nil {
				return err
			}
		}
		output.WriteByte(']')
	case 'o':
		keys := make([]string, 0, len(value.object))
		for key := range value.object {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		output.WriteByte('{')
		for index, key := range keys {
			if index != 0 {
				output.WriteByte(',')
			}
			encoded, err := json.Marshal(key)
			if err != nil {
				return err
			}
			output.Write(encoded)
			output.WriteByte(':')
			if err := emitEmbeddedValue(output, value.object[key], resourceLimits); err != nil {
				return err
			}
		}
		output.WriteByte('}')
	default:
		return io.ErrUnexpectedEOF
	}
	if resourceLimits && output.Len() > maxEmbeddedJSONBytes {
		return io.ErrShortBuffer
	}
	return nil
}

func normalizeJSONNumber(input string, resourceLimits bool) (string, error) {
	index := 0
	negative := false
	if strings.HasPrefix(input, "-") {
		negative = true
		index++
	}
	exponentIndex := strings.IndexAny(input[index:], "eE")
	mantissaEnd := len(input)
	exponent := int64(0)
	if exponentIndex >= 0 {
		exponentIndex += index
		mantissaEnd = exponentIndex
		parsed, err := strconv.ParseInt(input[exponentIndex+1:], 10, 32)
		if err != nil {
			return "", err
		}
		exponent = parsed
	}
	mantissa := input[index:mantissaEnd]
	dot := strings.IndexByte(mantissa, '.')
	integerDigits := len(mantissa)
	if dot >= 0 {
		integerDigits = dot
		mantissa = mantissa[:dot] + mantissa[dot+1:]
	}
	if mantissa == "" || strings.Trim(mantissa, "0123456789") != "" {
		return "", io.ErrUnexpectedEOF
	}
	if strings.Trim(mantissa, "0") == "" {
		return "0", nil
	}
	leading := len(mantissa) - len(strings.TrimLeft(mantissa, "0"))
	mantissa = strings.TrimLeft(mantissa, "0")
	decimalPosition := int64(integerDigits-leading) + exponent
	for strings.HasSuffix(mantissa, "0") && decimalPosition < int64(len(mantissa)) {
		mantissa = strings.TrimSuffix(mantissa, "0")
	}
	predicted := int64(len(mantissa))
	if decimalPosition <= 0 {
		predicted += 2 - decimalPosition
	} else if decimalPosition >= int64(len(mantissa)) {
		predicted = decimalPosition
	} else {
		predicted++
	}
	if resourceLimits && predicted > maxEmbeddedJSONBytes || predicted < 0 {
		return "", io.ErrShortBuffer
	}
	var output strings.Builder
	if negative {
		output.WriteByte('-')
	}
	switch {
	case decimalPosition <= 0:
		output.WriteString("0.")
		output.WriteString(strings.Repeat("0", int(-decimalPosition)))
		output.WriteString(mantissa)
	case decimalPosition >= int64(len(mantissa)):
		output.WriteString(mantissa)
		output.WriteString(strings.Repeat("0", int(decimalPosition)-len(mantissa)))
	default:
		output.WriteString(mantissa[:decimalPosition])
		output.WriteByte('.')
		output.WriteString(mantissa[decimalPosition:])
	}
	return output.String(), nil
}
