//go:build darwin || linux

package distribution

import (
	"bytes"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestPackIsDeterministicUnderRestrictiveUmask(t *testing.T) {
	previous := syscall.Umask(0o077)
	defer syscall.Umask(previous)

	first, err := Pack(syntheticPackageRequest(t, filepath.Join(t.TempDir(), "first")), DefaultPolicy())
	if err != nil {
		t.Fatal(err)
	}
	if first.Result.Status != StatusPass || len(first.Outputs) != 1 {
		t.Fatalf("first result = %#v, outputs = %#v", first.Result, first.Outputs)
	}
	firstArchive, err := os.ReadFile(first.Outputs[0].Path)
	if err != nil {
		t.Fatal(err)
	}

	second, err := Pack(syntheticPackageRequest(t, filepath.Join(t.TempDir(), "second")), DefaultPolicy())
	if err != nil {
		t.Fatal(err)
	}
	if second.Result.Status != StatusPass || len(second.Outputs) != 1 {
		t.Fatalf("second result = %#v, outputs = %#v", second.Result, second.Outputs)
	}
	secondArchive, err := os.ReadFile(second.Outputs[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	if first.Outputs[0].Digest != syntheticPackageDigest || second.Outputs[0].Digest != syntheticPackageDigest || !bytes.Equal(firstArchive, secondArchive) {
		t.Fatalf("restrictive umask changed deterministic package: first=%s second=%s equal=%t", first.Outputs[0].Digest, second.Outputs[0].Digest, bytes.Equal(firstArchive, secondArchive))
	}
}
