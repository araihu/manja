package store

import (
	"errors"
	"io"
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

func TestDurableAtomicWriteFuncRemovesStagingAfterCallbackFailure(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "fragment.html")
	want := errors.New("render failed")
	err := DurableAtomicWriteFunc(path, 0o600, func(writer io.Writer) error {
		if _, err := writer.Write([]byte("partial")); err != nil {
			return err
		}
		return want
	})
	if !errors.Is(err, want) {
		t.Fatalf("atomic write error = %v, want %v", err, want)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("partial destination exists: %v", err)
	}
	matches, err := filepath.Glob(filepath.Join(directory, atomicWriteStagingPattern))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("failed write left staging files: %#v", matches)
	}
}
