package application

import (
	"context"
	"errors"
	"testing"

	"github.com/araihu/manja/application/port"
	"github.com/araihu/manja/domain"
)

func TestRevisionServiceRejectsCorruptBlob(t *testing.T) {
	blobs := newSyncBlobFake(nil)
	key, err := blobs.Put(context.Background(), []byte("openapi: 3.1.0\n"))
	if err != nil {
		t.Fatal(err)
	}
	blobs.data[key] = []byte("corrupt")
	service, err := NewRevisionService(blobs)
	if err != nil {
		t.Fatal(err)
	}

	_, err = service.LoadSpec(context.Background(), domain.ContractRevision{ID: "revision-1", SpecBlobKey: string(key)})
	var appErr *Error
	if !errors.As(err, &appErr) || appErr.Kind != ErrorIntegrity {
		t.Fatalf("LoadSpec error = %#v, want integrity error", err)
	}
}

func TestRevisionServiceLoadsContentAddressedBlob(t *testing.T) {
	ctx := context.Background()
	blobs := newSyncBlobFake(nil)
	want := []byte("openapi: 3.1.0\n")
	key, err := blobs.Put(ctx, want)
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewRevisionService(blobs)
	if err != nil {
		t.Fatal(err)
	}
	got, err := service.LoadSpec(ctx, domain.ContractRevision{ID: "revision-1", SpecBlobKey: string(key)})
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("blob = %q, want %q", got, want)
	}
	if blobs.ctx != ctx || port.ContentAddressedBlobKey(got) != key {
		t.Fatal("revision load did not preserve context/content identity")
	}
}

func TestRevisionServiceRejectsNonCanonicalRevisionIdentityBeforeBlobRead(t *testing.T) {
	for _, revisionID := range []string{" revision-1 ", "revision\x00shadow", "revision-\xff"} {
		t.Run(revisionID, func(t *testing.T) {
			blobs := newSyncBlobFake(nil)
			service, err := NewRevisionService(blobs)
			if err != nil {
				t.Fatal(err)
			}
			_, err = service.LoadSpec(context.Background(), domain.ContractRevision{
				ID: revisionID, SpecBlobKey: string(port.ContentAddressedBlobKey([]byte("spec"))),
			})
			if err == nil {
				t.Fatal("LoadSpec accepted non-canonical revision identity")
			}
			if blobs.ctx != nil {
				t.Fatal("non-canonical revision identity reached blob store")
			}
		})
	}
}
