package catalog

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

const (
	maxSearchQueryBytes   = 256
	maxSearchQueryScalars = 128
	maxSearchQueryTokens  = 8
)

type normalizedSearchQuery struct {
	Exact  string
	Tokens []string
}

func normalizeSearchQuery(input string) (normalizedSearchQuery, error) {
	exact, err := normalizeSearchExact(input)
	if err != nil {
		return normalizedSearchQuery{}, err
	}
	return tokenizeNormalizedSearchExact(exact)
}

func tokenizeNormalizedSearchExact(exact string) (normalizedSearchQuery, error) {
	tokens := tokenizeSearchText(exact)
	if len(tokens) == 0 || len(tokens) > maxSearchQueryTokens {
		return normalizedSearchQuery{}, fmt.Errorf("%w: token count", ErrInvalidQuery)
	}
	return normalizedSearchQuery{Exact: exact, Tokens: tokens}, nil
}

func normalizeSearchExact(input string) (string, error) {
	if !utf8.ValidString(input) || len(input) > maxSearchQueryBytes {
		return "", fmt.Errorf("%w: encoding or byte limit", ErrInvalidQuery)
	}
	for _, character := range input {
		if unicode.IsControl(character) {
			return "", fmt.Errorf("%w: control character", ErrInvalidQuery)
		}
	}
	value := strings.TrimSpace(input)
	value = cases.Fold().String(norm.NFKC.String(value))
	if value == "" || len(value) > maxSearchQueryBytes || utf8.RuneCountInString(value) > maxSearchQueryScalars {
		return "", fmt.Errorf("%w: normalized limit", ErrInvalidQuery)
	}
	return value, nil
}

func tokenizeSearchText(value string) []string {
	var tokens []string
	var token strings.Builder
	flush := func() {
		if token.Len() > 0 {
			tokens = append(tokens, token.String())
			token.Reset()
		}
	}
	for _, character := range value {
		if unicode.IsLetter(character) || unicode.IsDigit(character) || strings.ContainsRune("/{}.:_-", character) {
			token.WriteRune(character)
		} else {
			flush()
		}
	}
	flush()
	return tokens
}
