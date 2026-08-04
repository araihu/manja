package source

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestCatalogReferenceCaptureAllowsInternalAndVendoredRelativeRefs(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeCatalogFile(t, root, "openapi.yaml", "openapi: 3.0.0\ninfo: {title: Test, version: v1}\npaths: {}\ncomponents:\n  schemas:\n    Local:\n      $ref: '#/components/schemas/Other'\n    Shared:\n      $ref: './common.yaml#/Shared'\n    Other: {type: string}\n")
	writeCatalogFile(t, root, "common.yaml", "Shared:\n  type: string\n")
	candidate, err := (FileCatalogSource{Root: root, Manifest: testCatalogManifest("strict-v1", "openapi.yaml")}).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(candidate.SupportFiles) != 1 || candidate.SupportFiles[0].SourcePath != "common.yaml" {
		t.Fatalf("support files = %#v", candidate.SupportFiles)
	}
}

func TestCatalogReferenceCaptureRejectsUnsafeOrMissingRefsWithoutNetwork(t *testing.T) {
	t.Parallel()

	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests.Add(1) }))
	defer server.Close()

	for name, reference := range map[string]string{
		"https":    server.URL + "/common.yaml",
		"file":     "file:///tmp/common.yaml",
		"absolute": "/tmp/common.yaml",
		"escape":   "../common.yaml",
		"missing":  "./missing.yaml",
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			writeCatalogFile(t, root, "openapi.yaml", fmt.Sprintf("openapi: 3.0.0\ninfo: {title: Test, version: v1}\npaths: {}\ncomponents:\n  schemas:\n    Shared:\n      $ref: %q\n", reference))
			if _, err := (FileCatalogSource{Root: root, Manifest: testCatalogManifest("strict-v1", "openapi.yaml")}).Load(context.Background()); err == nil {
				t.Fatal("unsafe or missing reference was accepted")
			}
		})
	}
	if requests.Load() != 0 {
		t.Fatalf("source acquisition made %d network requests", requests.Load())
	}
}
