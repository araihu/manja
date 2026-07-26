package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDurableAtomicWriteReplacesFileAndRemovesStaging(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "state.json")
	if err := durableAtomicWrite(path, []byte("first\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := durableAtomicWrite(path, []byte("second\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "second\n" {
		t.Fatalf("durable replacement = %q, want second", got)
	}
	matches, err := filepath.Glob(filepath.Join(directory, atomicWriteStagingPattern))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("durable replacement left staging files: %#v", matches)
	}
}
