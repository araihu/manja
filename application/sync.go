package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/araihu/manja/application/port"
	"github.com/araihu/manja/domain"
)

type SyncDependencies struct {
	Source     port.SourceFetcher
	Parser     port.Parser
	UnitOfWork port.UnitOfWork
	Blobs      port.BlobStore
	Clock      port.Clock
	Cache      port.Cache
}

type SyncService struct {
	source     port.SourceFetcher
	parser     port.Parser
	unitOfWork port.UnitOfWork
	blobs      port.BlobStore
	clock      port.Clock
	cache      port.Cache
}

type SyncCommand struct {
	ContractID string
	SourceID   string
	Trigger    string
}

type SyncResult struct {
	Spec     domain.SpecFile
	Revision domain.ContractRevision
	Index    domain.SpecIndex
	Record   domain.SyncRecord
	BlobKey  port.BlobKey
}

func NewSyncService(dependencies SyncDependencies) (*SyncService, error) {
	for _, required := range []struct {
		name  string
		value any
	}{
		{"source", dependencies.Source},
		{"parser", dependencies.Parser},
		{"unit of work", dependencies.UnitOfWork},
		{"blob store", dependencies.Blobs},
		{"clock", dependencies.Clock},
	} {
		if required.value == nil {
			return nil, dependencyError("construct sync service", required.name+" is required")
		}
	}
	return &SyncService{
		source: dependencies.Source, parser: dependencies.Parser,
		unitOfWork: dependencies.UnitOfWork, blobs: dependencies.Blobs,
		clock: dependencies.Clock, cache: dependencies.Cache,
	}, nil
}

func (s *SyncService) Sync(ctx context.Context, command SyncCommand) (SyncResult, error) {
	if err := validateSyncCommand(command); err != nil {
		return SyncResult{}, err
	}
	startedAt := s.clock.Now(ctx).UTC()
	spec, revision, err := s.source.Fetch(ctx)
	if err != nil {
		return SyncResult{}, s.recordFailure(ctx, command, spec, revision, startedAt, ErrorSource, "fetch source", err)
	}
	if revision.SourceID == "" {
		revision.SourceID = firstNonBlank(spec.SourceID, command.SourceID)
	}
	if spec.SourceID == "" {
		spec.SourceID = firstNonBlank(revision.SourceID, command.SourceID)
	}

	index, err := s.parser.Parse(ctx, spec, revision)
	if err != nil {
		return SyncResult{}, s.recordFailure(ctx, command, spec, revision, startedAt, ErrorParse, "parse source", err)
	}
	index.ProjectID = command.ContractID
	index.RevisionID = revision.ID
	snapshot := domain.NewContractSnapshot(command.ContractID, revision.ID, spec.Bytes, index)
	revision.ContractID = command.ContractID
	revision.SpecDigest = snapshot.SpecDigest
	revision.ContractDigest = snapshot.ContractDigest

	blobKey, err := s.blobs.Put(ctx, spec.Bytes)
	if err != nil {
		return SyncResult{}, wrapError(ErrorIntegrity, "write spec blob", err)
	}
	if !blobKey.Valid() || blobKey != port.ContentAddressedBlobKey(spec.Bytes) {
		return SyncResult{}, wrapError(ErrorIntegrity, "write spec blob", errors.New("blob store returned a non-content-addressed key"))
	}
	revision.SpecBlobKey = string(blobKey)
	finishedAt := s.clock.Now(ctx).UTC()
	record := successfulSyncRecord(command, spec, revision, startedAt, finishedAt)

	err = s.unitOfWork.Within(ctx, func(transactionContext context.Context, operational port.OperationalStore) error {
		if err := operational.SaveRevision(transactionContext, revision); err != nil {
			return fmt.Errorf("save revision: %w", err)
		}
		if err := operational.SaveSyncRecord(transactionContext, record); err != nil {
			return fmt.Errorf("save sync record: %w", err)
		}
		return nil
	})
	if err != nil {
		return SyncResult{}, wrapError(ErrorTransaction, "commit sync", err)
	}
	result := SyncResult{Spec: spec, Revision: revision, Index: index, Record: record, BlobKey: blobKey}
	if s.cache != nil {
		var cacheErr error
		for _, key := range []string{"public:" + command.ContractID, "search:" + command.ContractID + ":" + revision.ID} {
			if err := s.cache.Delete(ctx, key); err != nil {
				cacheErr = errors.Join(cacheErr, fmt.Errorf("invalidate %s: %w", key, err))
			}
		}
		if cacheErr != nil {
			return result, wrapError(ErrorCache, "invalidate sync cache entries", cacheErr)
		}
	}
	return result, nil
}

func (s *SyncService) recordFailure(
	ctx context.Context,
	command SyncCommand,
	spec domain.SpecFile,
	revision domain.ContractRevision,
	startedAt time.Time,
	kind ErrorKind,
	operation string,
	cause error,
) error {
	record := domain.SyncRecord{
		ID:           syncRecordID(command, revision, domain.SyncResultFailure),
		ProjectID:    command.ContractID,
		SourceID:     firstNonBlank(command.SourceID, revision.SourceID, spec.SourceID),
		RevisionID:   revision.ID,
		Trigger:      firstNonBlank(command.Trigger, "manual"),
		Ref:          revision.Ref,
		CommitSHA:    revision.CommitSHA,
		SpecPath:     spec.Path,
		Result:       domain.SyncResultFailure,
		ErrorSummary: errorSummary(cause),
		StartedAt:    startedAt,
		FinishedAt:   s.clock.Now(ctx).UTC(),
	}
	if err := s.unitOfWork.Within(ctx, func(transactionContext context.Context, operational port.OperationalStore) error {
		return operational.SaveSyncRecord(transactionContext, record)
	}); err != nil {
		return wrapError(ErrorTransaction, "record failed sync", errors.Join(cause, err))
	}
	return wrapError(kind, operation, cause)
}

func validateSyncCommand(command SyncCommand) error {
	if strings.TrimSpace(command.ContractID) == "" {
		return validationError("sync", "contract id is required")
	}
	if strings.TrimSpace(command.SourceID) == "" {
		return validationError("sync", "source id is required")
	}
	return nil
}

func successfulSyncRecord(command SyncCommand, spec domain.SpecFile, revision domain.ContractRevision, startedAt, finishedAt time.Time) domain.SyncRecord {
	return domain.SyncRecord{
		ID:         syncRecordID(command, revision, domain.SyncResultSuccess),
		ProjectID:  command.ContractID,
		SourceID:   firstNonBlank(command.SourceID, revision.SourceID, spec.SourceID),
		RevisionID: revision.ID,
		Trigger:    firstNonBlank(command.Trigger, "manual"),
		Ref:        revision.Ref,
		CommitSHA:  revision.CommitSHA,
		SpecPath:   spec.Path,
		Result:     domain.SyncResultSuccess,
		StartedAt:  startedAt,
		FinishedAt: finishedAt,
	}
}

func syncRecordID(command SyncCommand, revision domain.ContractRevision, result string) string {
	value := strings.Join([]string{command.ContractID, command.SourceID, firstNonBlank(command.Trigger, "manual"), revision.ID, revision.CommitSHA, result}, "\x00")
	sum := sha256.Sum256([]byte(value))
	return "sync-" + hex.EncodeToString(sum[:])[:24]
}

func errorSummary(err error) string {
	if err == nil {
		return ""
	}
	summary := strings.TrimSpace(err.Error())
	if len(summary) > 512 {
		return summary[:512]
	}
	return summary
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
