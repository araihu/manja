package projectionjson

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/araihu/manja/application/projection"
	"github.com/araihu/manja/domain"
)

func TestMarshalUsesCanonicalGoJSON(t *testing.T) {
	document := mustBuild(t, emptyFixture())
	got, err := Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	want := mustReadFixture(t, "v2-empty.json")
	if !bytes.Equal(got, want) {
		t.Fatalf("canonical bytes differ\ngot:  %s\nwant: %s", got, want)
	}
	if bytes.ContainsAny(got, "\r\n") || got[len(got)-1] != '}' {
		t.Fatalf("canonical bytes contain line ending or wrong suffix")
	}
}

func TestMarshalRejectsInvalidUTF8BeforeEncoding(t *testing.T) {
	document := mustBuild(t, emptyFixture())
	document.Title = string([]byte{0xff})
	bytes, err := Marshal(document)
	if err == nil || bytes != nil {
		t.Fatalf("Marshal = %q, %v; want nil bytes and error", bytes, err)
	}
}

func TestDigestHashesExactBytes(t *testing.T) {
	bytes := mustReadFixture(t, "v2-empty.json")
	want := strings.TrimSpace(string(mustReadFixture(t, "v2-empty.sha256")))
	if got := Digest(bytes); got != want {
		t.Fatalf("Digest = %q, want %q", got, want)
	}
	mutated := append([]byte(nil), bytes...)
	mutated = append(mutated, '\n')
	if Digest(mutated) == want {
		t.Fatal("digest ignored final newline")
	}
}

func TestErrorsDoNotDiscloseProjectionContent(t *testing.T) {
	sentinel := "__PRIVATE_PROJECTION_SENTINEL__"
	document := mustBuild(t, emptyFixture())
	document.ProjectID = sentinel + "\x00"
	assertCodecErrorSafe(t, func() error { _, err := Marshal(document); return err }, sentinel)

	unknown := []byte(`{"formatVersion":2,"` + sentinel + `":true}`)
	assertCodecErrorSafe(t, func() error { _, err := Unmarshal(unknown); return err }, sentinel)
	malformed := append([]byte(`{"formatVersion":2,"title":"`+sentinel), 0xff)
	assertCodecErrorSafe(t, func() error { _, err := Unmarshal(malformed); return err }, sentinel)
}

func TestErrorsAreBounded(t *testing.T) {
	document := mustBuild(t, emptyFixture())
	document.ProjectID = strings.Repeat("x", 1024) + "\x00"
	assertCodecErrorSafe(t, func() error { _, err := Marshal(document); return err }, strings.Repeat("x", 32))
}

func assertCodecErrorSafe(t *testing.T, operation func() error, sentinel string) {
	t.Helper()
	err := operation()
	if err == nil {
		t.Fatal("operation succeeded")
	}
	if strings.Contains(err.Error(), sentinel) || len(err.Error()) > 256 || !utf8.ValidString(err.Error()) {
		t.Fatalf("unsafe error %q", err)
	}
}

func mustMarshal(t *testing.T, document projection.Document) []byte {
	t.Helper()
	bytes, err := Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	return bytes
}

func mustBuild(t *testing.T, input domain.SpecIndex) projection.Document {
	t.Helper()
	document, err := (projection.Builder{}).Build(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	return document
}
