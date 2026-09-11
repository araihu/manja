package main

import (
	"bytes"
	"context"
	"encoding/json"
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
	if code != 0 || got.ConfigPath != "renderer.yaml" || got.DataDir != "data" || got.Output != "public" || got.BasePath != "/docs/" || got.SidebarChunkSize != 12 || got.FragmentWorkers != 4 || got.SchemaCacheBytes == nil || *got.SchemaCacheBytes != 128<<20 {
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

func TestExportSchemaCacheBudget(t *testing.T) {
	original := exportRenderer
	t.Cleanup(func() { exportRenderer = original })
	for _, tc := range []struct {
		value string
		bytes uint64
		code  int
	}{{"0", 0, 0}, {"256", 256 << 20, 0}, {"-1", 0, 2}, {"18446744073709551615", 0, 2}} {
		called := false
		exportRenderer = func(_ context.Context, o app.ExportOptions) (app.ExportReceipt, error) {
			called = true
			if o.SchemaCacheBytes == nil || *o.SchemaCacheBytes != tc.bytes {
				t.Fatal(o.SchemaCacheBytes)
			}
			return app.ExportReceipt{}, nil
		}
		var out, err bytes.Buffer
		code := run(context.Background(), []string{"export", "--renderer-config", "r", "--data-dir", "d", "--output", "o", "--base-path", "/", "--schema-cache-mib", tc.value}, &out, &err)
		if code != tc.code || called != (tc.code == 0) {
			t.Fatal(tc, code, called, err.String())
		}
	}
}

func TestExportCommandProgressModesWriteToStderr(t *testing.T) {
	original := exportRenderer
	t.Cleanup(func() { exportRenderer = original })
	exportRenderer = func(_ context.Context, options app.ExportOptions) (app.ExportReceipt, error) {
		if options.Progress == nil {
			t.Fatal("progress callback is nil")
		}
		options.Progress(app.ExportProgressEvent{SchemaVersion: 1, Event: "complete", Status: "success", Phase: "complete", Files: 2, Bytes: 3})
		return app.ExportReceipt{}, nil
	}
	for _, tc := range []struct {
		flag string
		want string
	}{
		{flag: "--progress", want: "event=complete"},
		{flag: "--progress=json", want: `"event":"complete"`},
	} {
		var stdout, stderr bytes.Buffer
		args := []string{"export", "--renderer-config", "r", "--data-dir", "d", "--output", "o", "--base-path", "/", tc.flag}
		if code := run(context.Background(), args, &stdout, &stderr); code != 0 {
			t.Fatalf("mode %s code=%d stderr=%q", tc.flag, code, stderr.String())
		}
		if !strings.Contains(stderr.String(), tc.want) {
			t.Fatalf("mode %s stderr=%q", tc.flag, stderr.String())
		}
		if tc.flag == "--progress=json" {
			var event app.ExportProgressEvent
			if err := json.Unmarshal([]byte(stderr.String()), &event); err != nil {
				t.Fatalf("json progress = %q: %v", stderr.String(), err)
			}
			if event.Event != "complete" || event.Files != 2 || event.Bytes != 3 {
				t.Fatalf("json event = %#v", event)
			}
		}
		if stdout.String() != "{\"schemaVersion\":0,\"basePath\":\"\",\"catalogs\":null,\"manifest\":\"\"}\n" {
			t.Fatalf("mode %s stdout=%q", tc.flag, stdout.String())
		}
	}
}

func TestExportCommandJSONProgressKeepsFailureStreamMachineReadable(t *testing.T) {
	original := exportRenderer
	t.Cleanup(func() { exportRenderer = original })
	exportRenderer = func(_ context.Context, options app.ExportOptions) (app.ExportReceipt, error) {
		options.Progress(app.ExportProgressEvent{SchemaVersion: 1, Event: "error", Status: "failure", Phase: "render", Error: "capture failed"})
		return app.ExportReceipt{}, errors.New("capture failed")
	}
	var stdout, stderr bytes.Buffer
	args := []string{"export", "--renderer-config", "r", "--data-dir", "d", "--output", "o", "--base-path", "/", "--progress=json"}
	if code := run(context.Background(), args, &stdout, &stderr); code != 1 {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	var event app.ExportProgressEvent
	if err := json.Unmarshal(stderr.Bytes(), &event); err != nil {
		t.Fatalf("stderr=%q: %v", stderr.String(), err)
	}
	if event.Event != "error" || event.Status != "failure" || event.Error != "capture failed" {
		t.Fatalf("event = %#v", event)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout=%q", stdout.String())
	}
}
