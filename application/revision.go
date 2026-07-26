package application

import (
	"context"
	"fmt"

	"github.com/araihu/manja/application/port"
	"github.com/araihu/manja/domain"
)

type RevisionService struct {
	blobs port.BlobStore
}

func NewRevisionService(blobs port.BlobStore) (*RevisionService, error) {
	if blobs == nil {
		return nil, dependencyError("construct revision service", "blob store is required")
	}
	return &RevisionService{blobs: blobs}, nil
}

func (s *RevisionService) LoadSpec(ctx context.Context, revision domain.ContractRevision) ([]byte, error) {
	if err := domain.ValidateCanonicalIdentity("revision id", revision.ID, false); err != nil {
		return nil, validationError("load revision", err.Error())
	}
	key := port.BlobKey(revision.SpecBlobKey)
	if !key.Valid() {
		return nil, wrapError(ErrorIntegrity, "load revision", fmt.Errorf("revision has invalid blob key %q", revision.SpecBlobKey))
	}
	data, err := s.blobs.Get(ctx, key)
	if err != nil {
		return nil, wrapError(ErrorIntegrity, "load revision blob", err)
	}
	if got := port.ContentAddressedBlobKey(data); got != key {
		return nil, wrapError(ErrorIntegrity, "load revision blob", fmt.Errorf("blob content key %q does not match revision key %q", got, key))
	}
	return data, nil
}
