//go:build windows

package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/araihu/manja/domain"
)

func TestWindowsRuntimeDurabilityWritesBlobStateAndMarker(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store := NewFileStore(root)
	key, err := store.Put(ctx, []byte("windows durable blob"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveRevision(ctx, domain.ContractRevision{
		ID: "revision", ContractID: "payments", SpecBlobKey: string(key),
	}); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		filepath.Join(root, "blobs", "sha256", string(key)[len("sha256:"):]),
		filepath.Join(root, "operational", "state.json"),
		filepath.Join(root, "operational", "schema.json"),
	} {
		if info, err := os.Stat(path); err != nil || info.Size() == 0 {
			t.Fatalf("durable Windows artifact %q: info=%v err=%v", path, info, err)
		}
	}
	if _, err := NewFileStore(root).ContractRevision(ctx, "payments", "revision"); err != nil {
		t.Fatalf("restart after Windows durable writes: %v", err)
	}
}
