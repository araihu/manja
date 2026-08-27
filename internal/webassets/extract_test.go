package webassets

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/araihu/assets/assetmeta"
)

func TestExtractRejectsUnsafeMembers(t *testing.T) {
	tests := []struct {
		name, header string
		typeflag     byte
	}{
		{"absolute", "/escape.js", tar.TypeReg},
		{"traversal", "package/../../escape.js", tar.TypeReg},
		{"symlink", "package/link", tar.TypeSymlink},
		{"hardlink", "package/link", tar.TypeLink},
		{"device", "package/device", tar.TypeChar},
		{"fifo", "package/fifo", tar.TypeFifo},
		{"outside package", "other/file.js", tar.TypeReg},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := extractArchive(context.Background(), archiveFixture(t, []tar.Header{{Name: test.header, Typeflag: test.typeflag, Size: 1}}, []string{"x"}), t.TempDir())
			if err == nil {
				t.Fatal("unsafe member accepted")
			}
		})
	}
}

func TestExtractRejectsDuplicateAndOversizedFiles(t *testing.T) {
	duplicate := []tar.Header{{Name: "package/a.js", Typeflag: tar.TypeReg, Size: 1}, {Name: "package/a.js", Typeflag: tar.TypeReg, Size: 1}}
	if err := extractArchive(context.Background(), archiveFixture(t, duplicate, []string{"a", "b"}), t.TempDir()); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("duplicate error = %v", err)
	}
	large := tar.Header{Name: "package/large.js", Typeflag: tar.TypeReg, Size: maxArchiveFileSize + 1}
	largeContents := strings.Repeat("x", int(maxArchiveFileSize+1))
	if err := extractArchive(context.Background(), archiveFixture(t, []tar.Header{large}, []string{largeContents}), t.TempDir()); err == nil || !strings.Contains(err.Error(), "file size") {
		t.Fatalf("large-file error = %v", err)
	}
}

func TestStagePreservesPublishedTreeOnFailure(t *testing.T) {
	root := t.TempDir()
	old := filepath.Join(root, "node_modules", "old", "index.js")
	if err := os.MkdirAll(filepath.Dir(old), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(old, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	packages := []Package{{Resource: "example", Name: "example", ArchiveRef: assetmeta.Ref{Resource: "example", Download: "archive"}}}
	opener := func(assetmeta.Ref) (io.ReadCloser, error) { return nil, errors.New("injected archive failure") }
	if _, err := stageWithOpener(context.Background(), root, packages, opener); err == nil {
		t.Fatal("Stage succeeded")
	}
	contents, err := os.ReadFile(old)
	if err != nil || string(contents) != "old" {
		t.Fatalf("published tree = %q, %v", contents, err)
	}
}

func TestStageRejectsPackageTraversalAndCancellation(t *testing.T) {
	opener := func(assetmeta.Ref) (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(nil)), nil }
	if _, err := stageWithOpener(context.Background(), t.TempDir(), []Package{{Name: "../escape"}}, opener); err == nil {
		t.Fatal("package traversal accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := stageWithOpener(ctx, t.TempDir(), []Package{{Name: "example"}}, opener); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation error = %v", err)
	}
}

func TestStageRealInventory(t *testing.T) {
	bundles, err := LoadRepositoryMetadata("../..")
	if err != nil {
		t.Fatal(err)
	}
	packages := allPackages(bundles)
	staged, err := Stage(context.Background(), t.TempDir(), packages)
	if err != nil {
		t.Fatal(err)
	}
	for _, pkg := range packages {
		if _, err := os.Stat(filepath.Join(staged, filepath.FromSlash(pkg.Name), "package.json")); err != nil {
			t.Fatalf("%s package.json: %v", pkg.Name, err)
		}
	}
	for _, required := range []string{"openapi-sampler/dist/openapi-sampler.js", "@readme/httpsnippet/dist/index.js"} {
		if _, err := os.Stat(filepath.Join(staged, filepath.FromSlash(required))); err != nil {
			t.Fatalf("required entry %s: %v", required, err)
		}
	}
}

func archiveFixture(t *testing.T, headers []tar.Header, contents []string) io.Reader {
	t.Helper()
	var output bytes.Buffer
	gz := gzip.NewWriter(&output)
	tw := tar.NewWriter(gz)
	for index := range headers {
		header := headers[index]
		if err := tw.WriteHeader(&header); err != nil {
			t.Fatal(err)
		}
		if index < len(contents) && header.Typeflag == tar.TypeReg {
			if _, err := tw.Write([]byte(contents[index])); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return bytes.NewReader(output.Bytes())
}
