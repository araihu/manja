package source

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestFileSourceFetchesSpec(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "openapi.yaml")
	if err := os.WriteFile(path, []byte("openapi: 3.1.0\ninfo:\n  title: T\n  version: v1\npaths: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	src := File{Path: path}
	spec, rev, err := src.Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if spec.Path != path || string(spec.Bytes) == "" {
		t.Fatalf("spec = %#v", spec)
	}
	if rev.Ref != "file" || rev.ID == "" {
		t.Fatalf("revision = %#v", rev)
	}
}
