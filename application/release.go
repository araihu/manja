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
	Revisions  port.RevisionReader
	UnitOfWork port.UnitOfWork
	Clock      port.Clock
}

type ReleaseService struct {
	revisions  port.RevisionReader
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
	if dependencies.Revisions == nil {
		return nil, dependencyError("construct release service", "revision reader is required")
	}
	if dependencies.UnitOfWork == nil {
		return nil, dependencyError("construct release service", "unit of work is required")
	}
	if dependencies.Clock == nil {
		return nil, dependencyError("construct release service", "clock is required")
	}
	return &ReleaseService{
		revisions: dependencies.Revisions, unitOfWork: dependencies.UnitOfWork, clock: dependencies.Clock,
	}, nil
}

func (s *ReleaseService) Coordinate(ctx context.Context, command ReleaseCommand) (ReleaseResult, error) {
	if err := validateReleaseCommand(command); err != nil {
		return ReleaseResult{}, err
	}
	baselineRevision, err := s.revisions.ContractRevision(ctx, command.ContractID, command.Review.BaselineRevisionID)
	if err != nil {
		return ReleaseResult{}, wrapError(ErrorIntegrity, "coordinate release", fmt.Errorf("load release review baseline: %w", err))
	}
	candidateRevision, err := s.revisions.ContractRevision(ctx, command.ContractID, command.RevisionID)
	if err != nil {
		return ReleaseResult{}, wrapError(ErrorIntegrity, "coordinate release", fmt.Errorf("load release review candidate: %w", err))
	}
	baselineSnapshot, err := revisionReviewSnapshot(baselineRevision)
	if err != nil {
		return ReleaseResult{}, wrapError(ErrorIntegrity, "coordinate release", fmt.Errorf("validate release review baseline: %w", err))
	}
	candidateSnapshot, err := revisionReviewSnapshot(candidateRevision)
	if err != nil {
		return ReleaseResult{}, wrapError(ErrorIntegrity, "coordinate release", fmt.Errorf("validate release review candidate: %w", err))
	}
	if err := validateContractReviewSnapshots(command.Review, baselineSnapshot, candidateSnapshot); err != nil {
		return ReleaseResult{}, wrapError(ErrorIntegrity, "coordinate release", fmt.Errorf("bind release review to persisted revisions: %w", err))
	}
	if err := domain.ValidateReleaseReviewReportAgainstSnapshots(
		command.Review.Report,
		command.ContractID,
		baselineSnapshot,
		candidateSnapshot,
	); err != nil {
		return ReleaseResult{}, wrapError(ErrorIntegrity, "coordinate release", fmt.Errorf("validate release review against persisted revisions: %w", err))
	}
	decision, err := releaseDecision(command.Review, command.Accepted)
	if err != nil {
		return ReleaseResult{}, wrapError(ErrorIntegrity, "coordinate release", fmt.Errorf("identify release decision: %w", err))
	}
	now := s.clock.Now(ctx).UTC()
	var next domain.ReleaseTrack
	err = s.unitOfWork.Within(ctx, func(transactionContext context.Context, operational port.OperationalStore) error {
		track, err := operational.ReleaseTrack(transactionContext, command.ContractID, command.TrackID)
		if err != nil {
			return fmt.Errorf("load release track: %w", err)
		}
		var changed bool
		next, changed, err = domain.ConsiderReleaseDecision(track, decision)
		if err != nil {
			return fmt.Errorf("apply release transition: %w", err)
		}
		if !changed {
			return nil
		}
		if track.CurrentRevisionID != baselineRevision.ID {
			return fmt.Errorf(
				"release track baseline %q does not match persisted review baseline %q",
				track.CurrentRevisionID,
				baselineRevision.ID,
			)
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
			ID: eventID, ContractID: command.ContractID, TrackID: command.TrackID, RevisionID: command.RevisionID,
			Kind: "release.track.considered", ActorID: command.ActorID, OccurredAt: now,
		}); err != nil {
			return fmt.Errorf("append audit event: %w", err)
		}
		if err := operational.Enqueue(transactionContext, domain.OutboxMessage{
			ID: "outbox-" + strings.TrimPrefix(eventID, "audit-"), ContractID: command.ContractID,
			TrackID: command.TrackID, RevisionID: command.RevisionID,
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

func releaseDecision(review domain.ContractReview, accepted bool) (domain.ReleaseDecision, error) {
	canonical, err := domain.CanonicalReviewJSON(review.Report)
	if err != nil {
		return domain.ReleaseDecision{}, err
	}
	digest := sha256.Sum256(canonical)
	return domain.ReleaseDecision{
		RevisionID:   review.CandidateRevisionID,
		ReviewID:     review.ID,
		ReviewDigest: hex.EncodeToString(digest[:]),
		Verdict:      review.Report.Verdict,
		Accepted:     accepted,
		EvaluatedAt:  review.Report.EvaluatedAt,
	}, nil
}

func revisionReviewSnapshot(revision domain.ContractRevision) (domain.ContractSnapshot, error) {
	if revision.ReviewSnapshot == nil {
		return domain.ContractSnapshot{}, fmt.Errorf("revision %q has no canonical review snapshot", revision.ID)
	}
	if err := domain.ValidateContractSnapshot(*revision.ReviewSnapshot); err != nil {
		return domain.ContractSnapshot{}, fmt.Errorf("revision %q canonical review snapshot: %w", revision.ID, err)
	}
	if revision.ReviewSnapshot.ContractID != revision.ContractID ||
		revision.ReviewSnapshot.RevisionID != revision.ID {
		return domain.ContractSnapshot{}, fmt.Errorf("revision %q canonical review snapshot identity does not match revision", revision.ID)
	}
	if revision.ReviewSnapshot.SpecDigest != revision.SpecDigest ||
		revision.ReviewSnapshot.ContractDigest != revision.ContractDigest {
		return domain.ContractSnapshot{}, fmt.Errorf("revision %q digests do not match its canonical review snapshot", revision.ID)
	}
	return *revision.ReviewSnapshot, nil
}

func validateContractReviewSnapshots(review domain.ContractReview, baseline, candidate domain.ContractSnapshot) error {
	if review.BaselineRevisionID != baseline.RevisionID ||
		review.BaselineSpecDigest != baseline.SpecDigest ||
		review.BaselineContractDigest != baseline.ContractDigest {
		return fmt.Errorf("review baseline evidence does not match persisted canonical snapshot")
	}
	if review.CandidateRevisionID != candidate.RevisionID ||
		review.CandidateSpecDigest != candidate.SpecDigest ||
		review.CandidateContractDigest != candidate.ContractDigest {
		return fmt.Errorf("review candidate evidence does not match persisted canonical snapshot")
	}
	return nil
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
