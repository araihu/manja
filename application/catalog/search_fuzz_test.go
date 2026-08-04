package catalog

import (
	"sort"
	"testing"
	"unicode/utf8"
)

func FuzzNormalizeSearchQuery(f *testing.F) {
	for _, seed := range []string{"PodSpec", "GET /api/v1/pods/{name}", "Ａpps StorageClass", "", "pod\x00spec"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input string) {
		query, err := normalizeSearchQuery(input)
		if err != nil {
			return
		}
		if !utf8.ValidString(query.Exact) || len(query.Exact) == 0 || len(query.Exact) > maxSearchQueryBytes || utf8.RuneCountInString(query.Exact) > maxSearchQueryScalars {
			t.Fatalf("accepted normalized query exceeds bounds: %#v", query)
		}
		if len(query.Tokens) == 0 || len(query.Tokens) > maxSearchQueryTokens {
			t.Fatalf("accepted token count exceeds bounds: %#v", query)
		}
	})
}

func FuzzSearchTrigrams(f *testing.F) {
	for _, seed := range []string{"deployment", "/apis/apps/v1", "界界界界", "ab"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input string) {
		trigrams := searchTrigrams(input)
		if len(trigrams) > maxSearchTrigramsPerToken || !sort.StringsAreSorted(trigrams) {
			t.Fatalf("invalid bounded trigrams: %#v", trigrams)
		}
		for index, trigram := range trigrams {
			if utf8.RuneCountInString(trigram) != 3 {
				t.Fatalf("trigram %q does not contain three scalars", trigram)
			}
			if index > 0 && trigrams[index-1] == trigram {
				t.Fatalf("duplicate trigram %q", trigram)
			}
		}
	})
}
