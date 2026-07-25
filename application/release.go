package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/araihu/manja/application/port"
	"github.com/araihu/manja/domain"
)

type ReleaseDependencies struct {
	UnitOfWork port.UnitOfWork
	Clock      port.Clock
}

type ReleaseService struct {
	unitOfWork port.UnitOfWork
	clock      port.Clock
}

type ReleaseCommand struct {
	ContractID string
	TrackID    string
	RevisionID string
	Accepted   bool
	Review     domain.ContractReview
	SyncRecord domain.SyncRecord
	PublicPath string
	ActorID    string
}

type ReleaseResult struct {
	Track domain.ReleaseTrack
}

func NewReleaseService(dependencies ReleaseDependencies) (*ReleaseService, error) {
	if dependencies.UnitOfWork == nil {
		return nil, dependencyError("construct release service", "unit of work is required")
	}
	if dependencies.Clock == nil {
		return nil, dependencyError("construct release service", "clock is required")
	}
	return &ReleaseService{unitOfWork: dependencies.UnitOfWork, clock: dependencies.Clock}, nil
}

func (s *ReleaseService) Coordinate(ctx context.Context, command ReleaseCommand) (ReleaseResult, error) {
	if err := validateReleaseCommand(command); err != nil {
		return ReleaseResult{}, err
	}
	now := s.clock.Now(ctx).UTC()
	var next domain.ReleaseTrack
	err := s.unitOfWork.Within(ctx, func(transactionContext context.Context, operational port.OperationalStore) error {
		track, err := operational.ReleaseTrack(transactionContext, command.ContractID, command.TrackID)
		if err != nil {
			return fmt.Errorf("load release track: %w", err)
		}
		next, err = domain.ConsiderReleaseRevision(track, command.RevisionID, command.Accepted)
		if err != nil {
			return fmt.Errorf("apply release transition: %w", err)
		}
		if err := operational.SaveReview(transactionContext, command.Review); err != nil {
			return fmt.Errorf("save review: %w", err)
		}
		if err := operational.SaveSyncRecord(transactionContext, command.SyncRecord); err != nil {
			return fmt.Errorf("save sync record: %w", err)
		}
		if err := operational.SaveReleaseTrack(transactionContext, track.Generation, next); err != nil {
			return fmt.Errorf("save release track: %w", err)
		}
		if command.Accepted && next.CurrentRevisionID == command.RevisionID {
			if err := operational.SavePublication(transactionContext, domain.Publication{
				ProjectID: command.ContractID, RevisionID: command.RevisionID, Public: true, Path: command.PublicPath,
			}); err != nil {
				return fmt.Errorf("save publication: %w", err)
			}
		}
		eventID := releaseEvidenceID(command, next.Generation)
		if err := operational.AppendAuditEvent(transactionContext, domain.AuditEvent{
			ID: eventID, ContractID: command.ContractID, Kind: "release.track.considered", ActorID: command.ActorID, OccurredAt: now,
		}); err != nil {
			return fmt.Errorf("append audit event: %w", err)
		}
		if err := operational.Enqueue(transactionContext, domain.OutboxMessage{
			ID: "outbox-" + strings.TrimPrefix(eventID, "audit-"), ContractID: command.ContractID,
			Topic: "release.track.updated", CreatedAt: now,
		}); err != nil {
			return fmt.Errorf("enqueue release event: %w", err)
		}
		return nil
	})
	if err != nil {
		return ReleaseResult{}, wrapError(ErrorTransaction, "coordinate release", err)
	}
	return ReleaseResult{Track: next}, nil
}

func validateReleaseCommand(command ReleaseCommand) error {
	for _, required := range []struct {
		name  string
		value string
	}{
		{"contract id", command.ContractID},
		{"track id", command.TrackID},
		{"revision id", command.RevisionID},
		{"review id", command.Review.ID},
		{"sync record id", command.SyncRecord.ID},
	} {
		if strings.TrimSpace(required.value) == "" {
			return validationError("coordinate release", required.name+" is required")
		}
	}
	if command.Review.ContractID != command.ContractID || command.Review.CandidateRevisionID != command.RevisionID {
		return validationError("coordinate release", "review identity does not match release command")
	}
	if command.SyncRecord.ProjectID != command.ContractID || command.SyncRecord.RevisionID != command.RevisionID {
		return validationError("coordinate release", "sync identity does not match release command")
	}
	if command.Accepted {
		if command.Review.Report.Verdict != domain.VerdictPass {
			return validationError("coordinate release", "accepted release requires a passing review")
		}
		if command.SyncRecord.Result != domain.SyncResultSuccess {
			return validationError("coordinate release", "accepted release requires a successful sync")
		}
		if strings.TrimSpace(command.PublicPath) == "" {
			return validationError("coordinate release", "accepted release requires a public path")
		}
	}
	return nil
}

func releaseEvidenceID(command ReleaseCommand, generation uint64) string {
	value := fmt.Sprintf("%s\x00%s\x00%s\x00%d\x00%t", command.ContractID, command.TrackID, command.RevisionID, generation, command.Accepted)
	sum := sha256.Sum256([]byte(value))
	return "audit-" + hex.EncodeToString(sum[:])[:24]
}
