package contracttest

import (
	"bytes"
	"context"
	"testing"

	"github.com/araihu/manja/application/port"
)

type BlobStoreFactory func(testing.TB) port.BlobStore

func BlobStore(t *testing.T, factory BlobStoreFactory) {
	t.Helper()
	if factory == nil {
		t.Fatal("blob store factory is required")
	}

	t.Run("content-addressed idempotent storage", func(t *testing.T) {
		store := factory(t)
		ctx := markedContext(t)
		input := []byte("openapi: 3.1.0\n")
		wantKey := port.ContentAddressedBlobKey(input)
		first, err := store.Put(ctx, input)
		if err != nil {
			t.Fatal(err)
		}
		replay, err := store.Put(ctx, append([]byte(nil), input...))
		if err != nil {
			t.Fatal(err)
		}
		if first != wantKey || replay != wantKey {
			t.Errorf("content-addressed replay keys = %q and %q, want %q", first, replay, wantKey)
		}
		input[0] = 'X'
		got, err := store.Get(ctx, wantKey)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, []byte("openapi: 3.1.0\n")) {
			t.Errorf("stored blob changed through caller input: %q", got)
		}
		got[0] = 'Y'
		reloaded, err := store.Get(ctx, wantKey)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(reloaded, []byte("openapi: 3.1.0\n")) {
			t.Errorf("stored blob changed through caller output: %q", reloaded)
		}
	})

	t.Run("honors cancellation", func(t *testing.T) {
		store := factory(t)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := store.Put(ctx, []byte("cancelled")); err == nil {
			t.Error("Put ignored cancelled context")
		}
		if _, err := store.Get(ctx, port.ContentAddressedBlobKey([]byte("cancelled"))); err == nil {
			t.Error("Get ignored cancelled context")
		}
	})
}
