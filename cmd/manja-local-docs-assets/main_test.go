package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteManifestIsDeterministicAndComplete(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "local-docs")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	for index, asset := range runtimeAssets {
		filename := filepath.Join(directory, asset.source)
		if err := os.MkdirAll(filepath.Dir(filename), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filename, bytes.Repeat([]byte{byte(index + 1)}, index+1), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	var first, second bytes.Buffer
	if err := writeManifest(&first, directory); err != nil {
		t.Fatal(err)
	}
	if err := writeManifest(&second, directory); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first.Bytes(), second.Bytes()) {
		t.Fatal("runtime asset manifest is not deterministic")
	}
	for _, asset := range runtimeAssets {
		if !bytes.Contains(first.Bytes(), []byte(asset.name)) {
			t.Errorf("manifest is missing %s", asset.name)
		}
	}
}
