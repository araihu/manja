package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	app "github.com/araihu/manja/internal/selfhosted"
)

func TestExportCommandWritesReceiptAndDisclosureWarning(t *testing.T) {
	original := exportRenderer
	t.Cleanup(func() { exportRenderer = original })
	var got app.ExportOptions
	exportRenderer = func(_ context.Context, options app.ExportOptions) (app.ExportReceipt, error) {
		got = options
		return app.ExportReceipt{SchemaVersion: 1, BasePath: "/docs/", Catalogs: []app.ExportCatalogReceipt{{CatalogID: "private", Mount: "/private", PublicationKey: "private", RevisionID: "revision", SnapshotID: "snapshot"}}, Manifest: "_manja/export.json"}, nil
	}
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"export", "--renderer-config", "renderer.yaml", "--data-dir", "data", "--output", "public", "--base-path", "/docs/"}, &stdout, &stderr)
	if code != 0 || got.ConfigPath != "renderer.yaml" || got.DataDir != "data" || got.Output != "public" || got.BasePath != "/docs/" {
		t.Fatalf("code=%d options=%#v stderr=%q", code, got, stderr.String())
	}
	if stdout.String() != "{\"schemaVersion\":1,\"basePath\":\"/docs/\",\"catalogs\":[{\"catalogId\":\"private\",\"mount\":\"/private\",\"publicationKey\":\"private\",\"revisionId\":\"revision\",\"snapshotId\":\"snapshot\"}],\"manifest\":\"_manja/export.json\"}\n" || !strings.Contains(stderr.String(), "every configured catalog") {
		t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestExportCommandRequiresAllCreationFlags(t *testing.T) {
	for _, args := range [][]string{{"export"}, {"export", "--renderer-config", "r", "--data-dir", "d", "--output", "o"}, {"export", "--renderer-config", "r", "--data-dir", "d", "--output", "o", "--base-path", "/", "extra"}} {
		var stdout, stderr bytes.Buffer
		if code := run(context.Background(), args, &stdout, &stderr); code != 2 {
			t.Errorf("run(%v) code=%d stderr=%q", args, code, stderr.String())
		}
	}
}

func TestExportVerifyRequiresOnlyOutputAndWritesReceipt(t *testing.T) {
	original := verifyExport
	t.Cleanup(func() { verifyExport = original })
	verifyExport = func(_ context.Context, output string) (app.ExportReceipt, error) {
		if output != "public" {
			t.Fatalf("output=%q", output)
		}
		return app.ExportReceipt{SchemaVersion: 1, BasePath: "/", Manifest: "_manja/export.json"}, nil
	}
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), []string{"export", "verify", "--output", "public"}, &stdout, &stderr); code != 0 {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
	if stdout.String() != "{\"schemaVersion\":1,\"basePath\":\"/\",\"catalogs\":null,\"manifest\":\"_manja/export.json\"}\n" || stderr.Len() != 0 {
		t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	var missingOut, missingErr bytes.Buffer
	if code := run(context.Background(), []string{"export", "verify"}, &missingOut, &missingErr); code != 2 {
		t.Fatalf("missing output code=%d", code)
	}
}

func TestExportCommandPreservesOperationalFailure(t *testing.T) {
	original := exportRenderer
	t.Cleanup(func() { exportRenderer = original })
	exportRenderer = func(context.Context, app.ExportOptions) (app.ExportReceipt, error) {
		return app.ExportReceipt{}, errors.New("capture failed")
	}
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"export", "--renderer-config", "r", "--data-dir", "d", "--output", "o", "--base-path", "/"}, &stdout, &stderr)
	if code != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "capture failed") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}
