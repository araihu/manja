package source

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/araihu/manja/contracttest"
	"github.com/araihu/manja/domain"
)

func TestFileSourcePublicContract(t *testing.T) {
	contracttest.SourceFetcher(t, func(t testing.TB) contracttest.SourceFixture {
		path := filepath.Join(t.TempDir(), "openapi.yaml")
		data := []byte("openapi: 3.1.0\ninfo:\n  title: Contract\n  version: v1\npaths: {}\n")
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(data)
		return contracttest.SourceFixture{
			Fetcher:      File{Path: path},
			WantSpec:     domain.SpecFile{Path: path, Format: "yaml", Bytes: data},
			WantRevision: domain.ContractRevision{ID: "file-" + hex.EncodeToString(digest[:])[:16], Ref: "file"},
		}
	})
}

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
