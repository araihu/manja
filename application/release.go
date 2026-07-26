package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/araihu/manja/application/port"
	"github.com/araihu/manja/domain"
)

type ReleaseDependencies struct {
	Revisions  port.RevisionReader
	Evidence   port.ReleaseEvidenceReader
	UnitOfWork port.UnitOfWork
	Clock      port.Clock
}

type ReleaseService struct {
	revisions  port.RevisionReader
	evidence   port.ReleaseEvidenceReader
	unitOfWork port.UnitOfWork
	clock      port.Clock
}

// ReleaseReviewMaxFutureSkew is the maximum connected or offline review clock
// lead accepted relative to the trusted release-service clock.
const ReleaseReviewMaxFutureSkew = 5 * time.Minute

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
	if dependencies.Evidence == nil {
		return nil, dependencyError("construct release service", "release evidence reader is required")
	}
	if dependencies.UnitOfWork == nil {
		return nil, dependencyError("construct release service", "unit of work is required")
	}
	if dependencies.Clock == nil {
		return nil, dependencyError("construct release service", "clock is required")
	}
	return &ReleaseService{
		revisions: dependencies.Revisions, evidence: dependencies.Evidence,
		unitOfWork: dependencies.UnitOfWork, clock: dependencies.Clock,
	}, nil
}

func (s *ReleaseService) Coordinate(ctx context.Context, command ReleaseCommand) (ReleaseResult, error) {
	if err := validateReleaseCommand(command); err != nil {
		return ReleaseResult{}, err
	}
	evidence, err := s.evidence.ReleaseEvidence(ctx, command.ContractID, command.TrackID, command.Review.ID)
	if err != nil {
		return ReleaseResult{}, wrapError(ErrorIntegrity, "coordinate release", fmt.Errorf("load persisted release evidence: %w", err))
	}
	if err := validateSelectedReleaseEvidence(command, evidence); err != nil {
		return ReleaseResult{}, wrapError(ErrorIntegrity, "coordinate release", err)
	}
	authorization := evidence.Authorization
	baselineRevision, err := s.revisions.ContractRevision(ctx, command.ContractID, authorization.BaselineRevisionID)
	if err != nil {
		return ReleaseResult{}, wrapError(ErrorIntegrity, "coordinate release", fmt.Errorf("load release review baseline: %w", err))
	}
	candidateRevision, err := s.revisions.ContractRevision(ctx, command.ContractID, authorization.CandidateRevisionID)
	if err != nil {
		return ReleaseResult{}, wrapError(ErrorIntegrity, "coordinate release", fmt.Errorf("load release review candidate: %w", err))
	}
	if err := validateReleaseRevisionIdentity(baselineRevision, authorization.ContractID, authorization.BaselineRevisionID, "", ""); err != nil {
		return ReleaseResult{}, wrapError(ErrorIntegrity, "coordinate release", fmt.Errorf("bind release baseline revision: %w", err))
	}
	if err := validateReleaseRevisionIdentity(
		candidateRevision,
		authorization.ContractID,
		authorization.CandidateRevisionID,
		authorization.SourceID,
		authorization.BoundRef,
	); err != nil {
		return ReleaseResult{}, wrapError(ErrorIntegrity, "coordinate release", fmt.Errorf("bind release candidate revision: %w", err))
	}
	baselineSnapshot, err := revisionReviewSnapshot(baselineRevision)
	if err != nil {
		return ReleaseResult{}, wrapError(ErrorIntegrity, "coordinate release", fmt.Errorf("validate release review baseline: %w", err))
	}
	candidateSnapshot, err := revisionReviewSnapshot(candidateRevision)
	if err != nil {
		return ReleaseResult{}, wrapError(ErrorIntegrity, "coordinate release", fmt.Errorf("validate release review candidate: %w", err))
	}
	if err := validateContractReviewSnapshots(evidence.Review, baselineSnapshot, candidateSnapshot); err != nil {
		return ReleaseResult{}, wrapError(ErrorIntegrity, "coordinate release", fmt.Errorf("bind release review to persisted revisions: %w", err))
	}
	if err := domain.ValidateReleaseReviewReportAgainstSnapshots(
		evidence.Review.Report,
		command.ContractID,
		baselineSnapshot,
		candidateSnapshot,
	); err != nil {
		return ReleaseResult{}, wrapError(ErrorIntegrity, "coordinate release", fmt.Errorf("validate release review against persisted revisions: %w", err))
	}
	now := s.clock.Now(ctx).UTC()
	if now.IsZero() {
		return ReleaseResult{}, wrapError(ErrorIntegrity, "coordinate release", fmt.Errorf("trusted release time is required"))
	}
	if evidence.Review.Report.EvaluatedAt.After(now.Add(ReleaseReviewMaxFutureSkew)) {
		return ReleaseResult{}, wrapError(
			ErrorIntegrity,
			"coordinate release",
			fmt.Errorf("release review evaluation time exceeds trusted clock skew"),
		)
	}
	if err := validateAppliedExceptionsAtReleaseTime(evidence.Review.Report, now); err != nil {
		return ReleaseResult{}, wrapError(ErrorIntegrity, "coordinate release", err)
	}
	decision, err := releaseDecision(evidence.Review, command.Accepted)
	if err != nil {
		return ReleaseResult{}, wrapError(ErrorIntegrity, "coordinate release", fmt.Errorf("identify release decision: %w", err))
	}
	var next domain.ReleaseTrack
	err = s.unitOfWork.Within(ctx, func(transactionContext context.Context, operational port.OperationalStore) error {
		track, err := operational.ReleaseTrack(transactionContext, command.ContractID, command.TrackID)
		if err != nil {
			return fmt.Errorf("load release track: %w", err)
		}
		if err := validateReleaseAuthorizationForTrack(authorization, track); err != nil {
			return err
		}
		var changed bool
		next, changed, err = domain.ConsiderReleaseDecision(track, decision)
		if err != nil {
			return fmt.Errorf("apply release transition: %w", err)
		}
		if !changed {
			return nil
		}
		if track.CurrentRevisionID != authorization.BaselineRevisionID {
			return fmt.Errorf(
				"release track baseline %q does not match authorized review baseline %q",
				track.CurrentRevisionID,
				authorization.BaselineRevisionID,
			)
		}
		if err := operational.SaveReleaseTrack(transactionContext, track.Generation, next); err != nil {
			return fmt.Errorf("save release track: %w", err)
		}
		if command.Accepted && next.CurrentRevisionID == authorization.CandidateRevisionID {
			if err := operational.SavePublication(transactionContext, domain.Publication{
				ProjectID: authorization.ContractID, RevisionID: authorization.CandidateRevisionID,
				Public: true, Path: authorization.PublicPath,
			}); err != nil {
				return fmt.Errorf("save publication: %w", err)
			}
		}
		eventID := releaseEvidenceID(authorization, command.Accepted, next.Generation)
		if err := operational.AppendAuditEvent(transactionContext, domain.AuditEvent{
			ID: eventID, ContractID: authorization.ContractID, TrackID: authorization.TrackID,
			RevisionID: authorization.CandidateRevisionID,
			Kind:       "release.track.considered", ActorID: command.ActorID, OccurredAt: now,
		}); err != nil {
			return fmt.Errorf("append audit event: %w", err)
		}
		if err := operational.Enqueue(transactionContext, domain.OutboxMessage{
			ID: "outbox-" + strings.TrimPrefix(eventID, "audit-"), ContractID: authorization.ContractID,
			TrackID: authorization.TrackID, RevisionID: authorization.CandidateRevisionID,
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

func validateAppliedExceptionsAtReleaseTime(report domain.ReviewReport, now time.Time) error {
	for comparisonIndex, comparison := range report.Comparisons {
		for exceptionIndex, exception := range comparison.Policy.AppliedExceptions {
			if !now.Before(exception.ExpiresAt) {
				return fmt.Errorf(
					"applied policy exception %d in comparison %d is expired at trusted release time",
					exceptionIndex,
					comparisonIndex,
				)
			}
		}
	}
	return nil
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
	for _, identity := range []struct {
		name       string
		value      string
		allowEmpty bool
	}{
		{name: "contract id", value: command.ContractID},
		{name: "track id", value: command.TrackID},
		{name: "revision id", value: command.RevisionID},
		{name: "review id", value: command.Review.ID},
		{name: "sync record id", value: command.SyncRecord.ID},
		{name: "actor id", value: command.ActorID, allowEmpty: true},
	} {
		if err := domain.ValidateCanonicalIdentity(identity.name, identity.value, identity.allowEmpty); err != nil {
			return validationError("coordinate release", err.Error())
		}
	}
	if err := domain.ValidateCanonicalPublicPath("public path", command.PublicPath, true); err != nil {
		return validationError("coordinate release", err.Error())
	}
	return nil
}

func validateSelectedReleaseEvidence(command ReleaseCommand, evidence domain.ReleaseEvidence) error {
	authorization := evidence.Authorization
	if err := domain.ValidateReleaseAuthorization(authorization); err != nil {
		return fmt.Errorf("validate persisted release authorization: %w", err)
	}
	if authorization.ContractID != command.ContractID ||
		authorization.TrackID != command.TrackID ||
		authorization.ReviewID != command.Review.ID ||
		authorization.SyncRecordID != command.SyncRecord.ID ||
		authorization.CandidateRevisionID != command.RevisionID {
		return fmt.Errorf("persisted release authorization does not match the selected command identity")
	}
	review := evidence.Review
	if review.ID != authorization.ReviewID ||
		review.ContractID != authorization.ContractID ||
		review.BaselineRevisionID != authorization.BaselineRevisionID ||
		review.CandidateRevisionID != authorization.CandidateRevisionID ||
		review.Report.PolicyDigest != authorization.PolicyDigest {
		return fmt.Errorf("persisted review does not match its track authorization")
	}
	syncRecord := evidence.SyncRecord
	if syncRecord.ID != authorization.SyncRecordID ||
		syncRecord.ProjectID != authorization.ContractID ||
		syncRecord.SourceID != authorization.SourceID ||
		syncRecord.RevisionID != authorization.CandidateRevisionID ||
		syncRecord.Ref != authorization.BoundRef {
		return fmt.Errorf("persisted sync does not match its track authorization")
	}
	if command.Accepted {
		if review.Report.Verdict != domain.VerdictPass {
			return fmt.Errorf("accepted release requires a passing persisted review")
		}
		if syncRecord.Result != domain.SyncResultSuccess {
			return fmt.Errorf("accepted release requires a successful persisted sync")
		}
	}
	return nil
}

func validateReleaseRevisionIdentity(
	revision domain.ContractRevision,
	contractID, revisionID, sourceID, boundRef string,
) error {
	if revision.ContractID != contractID || revision.ID != revisionID {
		return fmt.Errorf("loaded revision identity does not match its authorized identity")
	}
	if sourceID != "" && revision.SourceID != sourceID {
		return fmt.Errorf("loaded revision source does not match its authorized source")
	}
	if boundRef != "" && revision.Ref != boundRef {
		return fmt.Errorf("loaded revision ref does not match its authorized ref")
	}
	return nil
}

func validateReleaseAuthorizationForTrack(authorization domain.ReleaseAuthorization, track domain.ReleaseTrack) error {
	if track.ContractID != authorization.ContractID || track.ID != authorization.TrackID {
		return fmt.Errorf("release authorization belongs to another track")
	}
	if track.BoundRef != authorization.BoundRef {
		return fmt.Errorf("release authorization ref does not match the persisted track")
	}
	return nil
}

func releaseEvidenceID(authorization domain.ReleaseAuthorization, accepted bool, generation uint64) string {
	value := fmt.Sprintf(
		"%s\x00%s\x00%s\x00%s\x00%d\x00%t",
		authorization.ContractID,
		authorization.TrackID,
		authorization.CandidateRevisionID,
		authorization.ReviewID,
		generation,
		accepted,
	)
	sum := sha256.Sum256([]byte(value))
	return "audit-" + hex.EncodeToString(sum[:])[:24]
}
