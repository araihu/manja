package projectionjson

import (
	"bytes"
	"strings"
	"testing"
)

func TestUnmarshalRejectsDuplicateKeysAtEveryDepth(t *testing.T) {
	cases := [][]byte{
		[]byte(`{"formatVersion":2,"formatVersion":2}`),
		[]byte(`{"formatVersion":2,"projectId":"p","revisionId":"r","title":"","apiVersion":"","branding":{"displayName":"","displayName":""}}`),
		[]byte(`{"formatVersion":2,"projectId":"p","revisionId":"r","title":"","apiVersion":"","branding":{},"overview":{"contact":{"name":"","name":""}}}`),
	}
	for index, input := range cases {
		if document, err := Unmarshal(input); err == nil || document.FormatVersion != 0 {
			t.Errorf("case %d accepted: %#v", index, document)
		}
	}
}

func TestUnmarshalRejectsUnknownFieldsAtEveryLayer(t *testing.T) {
	base := string(mustReadFixture(t, "v2-operation.json"))
	fragments := []string{
		`"formatVersion":2`,
		`"displayName":""`,
		`"anchor":"overview"`,
		`"id":"main-content"`,
		`"ordinal":0,"id":"operation-tag-`,
		`"ordinal":0,"id":"operation-list/pets"`,
		`"ordinal":0,"path":"/"`,
	}
	for _, fragment := range fragments {
		position := strings.Index(base, fragment)
		if position < 0 {
			t.Fatalf("fixture lacks layer fragment %q", fragment)
		}
		end := position + len(fragment)
		mutated := base[:end] + `,"unknownSentinel":0` + base[end:]
		if _, err := Unmarshal([]byte(mutated)); err == nil {
			t.Errorf("unknown field at fragment %q accepted", fragment)
		}
	}
}

func TestUnmarshalRejectsTrailingValues(t *testing.T) {
	for _, suffix := range []string{"{}", " true", "\n"} {
		input := append(append([]byte(nil), mustReadFixture(t, "v2-empty.json")...), suffix...)
		if _, err := Unmarshal(input); err == nil {
			t.Errorf("trailing %q accepted", suffix)
		}
	}
}

func TestUnmarshalRejectsNonCanonicalBytes(t *testing.T) {
	canonical := mustReadFixture(t, "v2-empty.json")
	mutations := [][]byte{
		append([]byte(" "), canonical...),
		append(append([]byte(nil), canonical...), '\n'),
		bytes.Replace(canonical, []byte(`"formatVersion":2`), []byte(`"formatVersion": 2`), 1),
		bytes.Replace(canonical, []byte(`\u003c`), []byte(`<`), 1),
		bytes.Replace(canonical, []byte(`"sidebarSections":[]`), []byte(`"sidebarSections":null`), 1),
	}
	for index, input := range mutations {
		if _, err := Unmarshal(input); err == nil {
			t.Errorf("noncanonical case %d accepted", index)
		}
	}
}

func TestUnmarshalRejectsInvalidWireNumbers(t *testing.T) {
	for _, token := range []string{"-1", "1.0", "1e0", "01", "4294967296"} {
		input := bytes.Replace(mustReadFixture(t, "v2-empty.json"), []byte(`"formatVersion":2`), []byte(`"formatVersion":`+token), 1)
		if _, err := Unmarshal(input); err == nil {
			t.Errorf("wire number %q accepted", token)
		}
	}
}
