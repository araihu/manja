package projection

import (
	"strings"
	"testing"
)

func FuzzEmbeddedJSON(f *testing.F) {
	for _, seed := range []string{
		`{"z":1e+03,"a":1e-3}`,
		`{"a":1,"a":2}`,
		`true`,
		`-0`,
		strings.Repeat("[", 65) + "0" + strings.Repeat("]", 65),
		string([]byte{0xff}),
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input string) {
		if len(input) > 64*1024 {
			t.Skip()
		}
		canonical, err := canonicalEmbeddedJSON(input)
		if err != nil {
			return
		}
		if len(canonical) > maxEmbeddedJSONBytes {
			t.Fatalf("accepted output exceeds bound: %d", len(canonical))
		}
		repeated, err := canonicalEmbeddedJSON(canonical)
		if err != nil {
			t.Fatalf("canonical output rejected: %v", err)
		}
		if repeated != canonical {
			t.Fatalf("canonicalization not idempotent: %q != %q", repeated, canonical)
		}
	})
}
