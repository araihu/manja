package catalog

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestNormalizeSearchQueryUsesNFKCCaseFoldAndPreservedPathPunctuation(t *testing.T) {
	t.Parallel()

	query, err := normalizeSearchQuery("  ＧＥＴ /Apis/{Name}:Watch_v1-Test  ")
	if err != nil {
		t.Fatal(err)
	}
	if query.Exact != "get /apis/{name}:watch_v1-test" {
		t.Fatalf("exact = %q", query.Exact)
	}
	if len(query.Tokens) != 2 || query.Tokens[0] != "get" || query.Tokens[1] != "/apis/{name}:watch_v1-test" {
		t.Fatalf("tokens = %#v", query.Tokens)
	}
}

func TestNormalizeSearchQueryRejectsEveryRequestBound(t *testing.T) {
	t.Parallel()

	invalidUTF8 := string([]byte{'p', 0xff})
	for name, query := range map[string]string{
		"empty":        "   ",
		"invalid UTF8": invalidUTF8,
		"bytes":        strings.Repeat("a", 257),
		"scalars":      strings.Repeat("界", 129),
		"tokens":       "one two three four five six seven eight nine",
		"expansion":    strings.Repeat("İ", 85),
		"NUL":          "pod\x00spec",
		"control":      "pod\nspec",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := normalizeSearchQuery(query); err == nil {
				t.Fatal("invalid query was accepted")
			}
		})
	}
	if !utf8.ValidString(strings.Repeat("界", 128)) {
		t.Fatal("test fixture is not valid UTF-8")
	}
	maximum, err := normalizeSearchQuery(strings.Repeat("a", maxSearchQueryScalars))
	if err != nil || len(maximum.Tokens) != 1 {
		t.Fatalf("maximum scalar query = %#v, %v", maximum, err)
	}
}

func TestSearchIndexAddsCamelCaseAndDigitWordTokens(t *testing.T) {
	t.Parallel()

	tokens := searchTokenSet("StorageClass", "listAppsV1Deployment")
	for _, want := range []string{"storage", "class", "list", "apps", "v", "1", "deployment", "storageclass", "listappsv1deployment"} {
		found := false
		for _, token := range tokens {
			if token == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("token %q missing from %#v", want, tokens)
		}
	}
}
