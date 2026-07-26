package domain

import (
	"fmt"
	"path"
	"reflect"
	"strings"
	"time"
)

type ReleaseMode string

const (
	ReleaseModePinned    ReleaseMode = "pinned"
	ReleaseModeFollowing ReleaseMode = "following"
)

type ReleaseTrack struct {
	ID                  string           `json:"id"`
	ContractID          string           `json:"contractId"`
	BoundRef            string           `json:"boundRef,omitempty"`
	Mode                ReleaseMode      `json:"mode"`
	Generation          uint64           `json:"generation"`
	CurrentRevisionID   string           `json:"currentRevisionId,omitempty"`
	CandidateRevisionID string           `json:"candidateRevisionId,omitempty"`
	LastDecision        *ReleaseDecision `json:"lastDecision,omitempty"`
}

type ReleaseDecision struct {
	RevisionID   string    `json:"revisionId"`
	ReviewID     string    `json:"reviewId"`
	ReviewDigest string    `json:"reviewDigest"`
	Verdict      string    `json:"verdict"`
	Accepted     bool      `json:"accepted"`
	EvaluatedAt  time.Time `json:"evaluatedAt,omitempty"`
}

// ReleaseAuthorization is the deployment-owned, immutable binding between one
// canonical review and exactly one release track, sync result, ref, route, and
// effective policy identity.
type ReleaseAuthorization struct {
	ContractID          string `json:"contractId"`
	TrackID             string `json:"trackId"`
	ReviewID            string `json:"reviewId"`
	SyncRecordID        string `json:"syncRecordId"`
	BaselineRevisionID  string `json:"baselineRevisionId"`
	CandidateRevisionID string `json:"candidateRevisionId"`
	SourceID            string `json:"sourceId"`
	BoundRef            string `json:"boundRef"`
	PublicPath          string `json:"publicPath"`
	PolicyDigest        string `json:"policyDigest"`
}

// ReleaseEvidence is the immutable review and sync bundle selected by a
// deployment-owned authorization. Release orchestration loads this bundle from
// persistence instead of trusting caller-supplied report or sync fields.
type ReleaseEvidence struct {
	Authorization ReleaseAuthorization `json:"authorization"`
	Review        ContractReview       `json:"review"`
	SyncRecord    SyncRecord           `json:"syncRecord"`
}

// ValidateReleaseAuthorization rejects ambiguous or caller-normalized
// identities before the authorization becomes durable release evidence.
func ValidateReleaseAuthorization(authorization ReleaseAuthorization) error {
	for _, identity := range []struct {
		name  string
		value string
	}{
		{"release authorization contract id", authorization.ContractID},
		{"release authorization track id", authorization.TrackID},
		{"release authorization review id", authorization.ReviewID},
		{"release authorization sync record id", authorization.SyncRecordID},
		{"release authorization baseline revision id", authorization.BaselineRevisionID},
		{"release authorization candidate revision id", authorization.CandidateRevisionID},
		{"release authorization source id", authorization.SourceID},
		{"release authorization bound ref", authorization.BoundRef},
	} {
		if err := validateCanonicalReleaseIdentity(identity.name, identity.value, false); err != nil {
			return err
		}
	}
	if authorization.PublicPath == "" ||
		authorization.PublicPath != strings.TrimSpace(authorization.PublicPath) ||
		!strings.HasPrefix(authorization.PublicPath, "/") ||
		strings.Contains(authorization.PublicPath, `\`) ||
		path.Clean(authorization.PublicPath) != authorization.PublicPath {
		return fmt.Errorf("release authorization public path is invalid")
	}
	if !isLowerSHA256(authorization.PolicyDigest) {
		return fmt.Errorf("release authorization policy digest must be lowercase SHA-256")
	}
	return nil
}

// ConsiderReleaseRevision is the source-compatible boolean decision helper.
// It allows rejection followed by acceptance, but once its synthetic evidence
// accepts a candidate it cannot revoke or supersede that acceptance because
// the legacy signature carries no authoritative identity or evaluation time.
// Reversible ordered decisions must use ConsiderReleaseDecision.
func ConsiderReleaseRevision(track ReleaseTrack, revisionID string, accepted bool) (ReleaseTrack, error) {
	if err := ValidateReleaseTrack(track); err != nil {
		return ReleaseTrack{}, err
	}
	if err := validateCanonicalReleaseIdentity("revision id", revisionID, false); err != nil {
		return ReleaseTrack{}, err
	}
	if track.LastDecision != nil {
		if track.LastDecision.ReviewID != "legacy-release-decision" {
			return ReleaseTrack{}, fmt.Errorf("legacy release helper cannot supersede an authoritative decision")
		}
		if track.LastDecision.Accepted {
			if accepted && track.LastDecision.RevisionID == revisionID {
				return track, nil
			}
			return ReleaseTrack{}, fmt.Errorf("legacy accepted decision cannot be revoked or superseded")
		}
	}

	verdict := VerdictFail
	if accepted {
		verdict = VerdictPass
	}
	decision := ReleaseDecision{
		RevisionID:   revisionID,
		ReviewID:     "legacy-release-decision",
		ReviewDigest: sha256Hex([]byte(fmt.Sprintf("%s\x00%t", revisionID, accepted))),
		Verdict:      verdict,
		Accepted:     accepted,
		EvaluatedAt:  time.Unix(0, 0).UTC(),
	}
	if track.LastDecision != nil {
		if sameReleaseDecisionWithoutTime(*track.LastDecision, decision) {
			decision.EvaluatedAt = track.LastDecision.EvaluatedAt
		} else if !track.LastDecision.EvaluatedAt.IsZero() {
			decision.EvaluatedAt = track.LastDecision.EvaluatedAt.Add(time.Nanosecond)
		}
	}
	next, _, err := ConsiderReleaseDecision(track, decision)
	return next, err
}

func sameReleaseDecisionWithoutTime(left, right ReleaseDecision) bool {
	left.EvaluatedAt = time.Time{}
	right.EvaluatedAt = time.Time{}
	return left == right
}

// ConsiderReleaseDecision applies a reviewed decision to one track. Replay
// identity includes the review bytes, verdict, acceptance, and evaluation time.
// Strictly older decisions are stale no-ops; only newer decisions may apply.
func ConsiderReleaseDecision(track ReleaseTrack, decision ReleaseDecision) (ReleaseTrack, bool, error) {
	if err := ValidateReleaseTrack(track); err != nil {
		return ReleaseTrack{}, false, err
	}
	if err := validateReleaseDecision(decision); err != nil {
		return ReleaseTrack{}, false, err
	}
	if track.LastDecision != nil && *track.LastDecision == decision {
		return track, false, nil
	}
	if track.Generation == ^uint64(0) {
		return ReleaseTrack{}, false, fmt.Errorf("release track generation is exhausted")
	}
	if decision.EvaluatedAt.IsZero() {
		return ReleaseTrack{}, false, fmt.Errorf("release decision evaluation time is required")
	}
	if track.LastDecision != nil && !track.LastDecision.EvaluatedAt.IsZero() {
		switch {
		case decision.EvaluatedAt.Before(track.LastDecision.EvaluatedAt):
			return track, false, nil
		case decision.EvaluatedAt.Equal(track.LastDecision.EvaluatedAt):
			return ReleaseTrack{}, false, fmt.Errorf("different release decisions have the same evaluation time")
		}
	}

	next := track
	next.Generation++
	next.LastDecision = &decision
	if !decision.Accepted || track.Mode == ReleaseModePinned {
		next.CandidateRevisionID = decision.RevisionID
		return next, true, nil
	}
	next.CurrentRevisionID = decision.RevisionID
	next.CandidateRevisionID = ""
	return next, true, nil
}

// PromoteReleaseRevision explicitly advances a pinned track to its recorded
// candidate only when its latest persisted decision accepted that revision.
// Replaying an already-applied accepted promotion is a no-op.
func PromoteReleaseRevision(track ReleaseTrack, revisionID string) (ReleaseTrack, error) {
	if err := ValidateReleaseTrack(track); err != nil {
		return ReleaseTrack{}, err
	}
	if track.Mode != ReleaseModePinned {
		return ReleaseTrack{}, fmt.Errorf("only pinned tracks require promotion")
	}
	if err := validateCanonicalReleaseIdentity("revision id", revisionID, false); err != nil {
		return ReleaseTrack{}, err
	}
	if track.LastDecision == nil ||
		!track.LastDecision.Accepted ||
		track.LastDecision.Verdict != VerdictPass ||
		track.LastDecision.RevisionID != revisionID {
		return ReleaseTrack{}, fmt.Errorf("revision %q has no matching accepted decision", revisionID)
	}
	if track.CandidateRevisionID == "" && track.CurrentRevisionID == revisionID {
		return track, nil
	}
	if revisionID != track.CandidateRevisionID {
		return ReleaseTrack{}, fmt.Errorf("revision %q is not the track candidate", revisionID)
	}
	if track.Generation == ^uint64(0) {
		return ReleaseTrack{}, fmt.Errorf("release track generation is exhausted")
	}
	next := track
	next.Generation++
	next.CurrentRevisionID = revisionID
	next.CandidateRevisionID = ""
	return next, nil
}

// CloneReleaseTrack returns an isolated copy of a release track, including its
// pointer-backed decision evidence. Store adapters must use it at read and
// write boundaries so callers cannot mutate persisted authorization state.
func CloneReleaseTrack(track ReleaseTrack) ReleaseTrack {
	cloned := track
	if track.LastDecision != nil {
		decision := *track.LastDecision
		cloned.LastDecision = &decision
	}
	return cloned
}

// ValidateReleaseTrack verifies the complete persisted release authorization
// state. Legacy tracks without decision evidence remain valid only when they
// do not claim a pending candidate.
func ValidateReleaseTrack(track ReleaseTrack) error {
	if err := validateCanonicalReleaseIdentity("release track id", track.ID, false); err != nil {
		return err
	}
	if err := validateCanonicalReleaseIdentity("release track contract id", track.ContractID, false); err != nil {
		return err
	}
	if err := validateCanonicalReleaseIdentity("release track bound ref", track.BoundRef, true); err != nil {
		return err
	}
	if err := validateCanonicalReleaseIdentity("release track current revision id", track.CurrentRevisionID, true); err != nil {
		return err
	}
	if err := validateCanonicalReleaseIdentity("release track candidate revision id", track.CandidateRevisionID, true); err != nil {
		return err
	}
	if track.Mode != ReleaseModePinned && track.Mode != ReleaseModeFollowing {
		return fmt.Errorf("unsupported release track mode %q", track.Mode)
	}
	if track.LastDecision == nil {
		if track.CandidateRevisionID != "" {
			return fmt.Errorf("release track candidate requires decision evidence")
		}
		return nil
	}
	if err := validateReleaseDecision(*track.LastDecision); err != nil {
		return fmt.Errorf("invalid last release decision: %w", err)
	}
	if track.LastDecision.EvaluatedAt.IsZero() {
		return fmt.Errorf("last release decision evaluation time is required")
	}
	if track.CandidateRevisionID != "" {
		if track.CandidateRevisionID != track.LastDecision.RevisionID {
			return fmt.Errorf("release track candidate does not match last decision")
		}
		return nil
	}
	if !track.LastDecision.Accepted {
		return fmt.Errorf("rejected release decision requires its candidate")
	}
	if track.CurrentRevisionID != track.LastDecision.RevisionID {
		return fmt.Errorf("release track current revision does not match last accepted decision")
	}
	return nil
}

// ValidateReleaseTrackTransition accepts only a complete domain transition:
// an exact no-op, a newly considered authoritative decision, or a pinned
// promotion derived from the current accepted candidate.
func ValidateReleaseTrackTransition(current, next ReleaseTrack) error {
	if err := ValidateReleaseTrack(current); err != nil {
		return fmt.Errorf("invalid current release track: %w", err)
	}
	if err := ValidateReleaseTrack(next); err != nil {
		return fmt.Errorf("invalid next release track: %w", err)
	}
	if current.ID != next.ID || current.ContractID != next.ContractID {
		return fmt.Errorf("release track identity cannot change")
	}
	if reflect.DeepEqual(current, next) {
		return nil
	}
	if current.LastDecision != nil && next.LastDecision != nil && *current.LastDecision == *next.LastDecision {
		if current.Mode == ReleaseModePinned && current.CandidateRevisionID != "" {
			promoted, err := PromoteReleaseRevision(current, current.CandidateRevisionID)
			if err == nil && reflect.DeepEqual(promoted, next) {
				return nil
			}
		}
		return fmt.Errorf("persisted release track mutation is not an exact no-op or pinned promotion")
	}
	if next.LastDecision == nil {
		return fmt.Errorf("release track decision evidence cannot be removed")
	}
	expected, changed, err := ConsiderReleaseDecision(current, *next.LastDecision)
	if err != nil {
		return fmt.Errorf("validate persisted release decision: %w", err)
	}
	if !changed || !reflect.DeepEqual(expected, next) {
		return fmt.Errorf("persisted release track mutation does not match the authoritative decision")
	}
	return nil
}

func validateReleaseDecision(decision ReleaseDecision) error {
	if err := validateCanonicalReleaseIdentity("release decision revision id", decision.RevisionID, false); err != nil {
		return err
	}
	if err := validateCanonicalReleaseIdentity("release decision review id", decision.ReviewID, false); err != nil {
		return err
	}
	if !isLowerSHA256(decision.ReviewDigest) {
		return fmt.Errorf("release decision review digest must be lowercase SHA-256")
	}
	if decision.Verdict != VerdictPass && decision.Verdict != VerdictFail {
		return fmt.Errorf("unsupported release decision verdict %q", decision.Verdict)
	}
	if decision.Accepted && decision.Verdict != VerdictPass {
		return fmt.Errorf("accepted release decision requires a passing verdict")
	}
	if !decision.EvaluatedAt.IsZero() && decision.EvaluatedAt.Location() != time.UTC {
		return fmt.Errorf("release decision evaluation time must be UTC")
	}
	return nil
}

func validateCanonicalReleaseIdentity(name, value string, allowEmpty bool) error {
	if value == "" {
		if allowEmpty {
			return nil
		}
		return fmt.Errorf("%s is required", name)
	}
	if value != strings.TrimSpace(value) {
		return fmt.Errorf("%s must not contain leading or trailing whitespace", name)
	}
	return nil
}

type ContractReview struct {
	ID                      string       `json:"id"`
	ContractID              string       `json:"contractId"`
	BaselineRevisionID      string       `json:"baselineRevisionId"`
	BaselineSpecDigest      string       `json:"baselineSpecDigest"`
	BaselineContractDigest  string       `json:"baselineContractDigest"`
	CandidateRevisionID     string       `json:"candidateRevisionId"`
	CandidateSpecDigest     string       `json:"candidateSpecDigest"`
	CandidateContractDigest string       `json:"candidateContractDigest"`
	Report                  ReviewReport `json:"report"`
}

type AuditEvent struct {
	ID         string    `json:"id"`
	ContractID string    `json:"contractId"`
	TrackID    string    `json:"trackId,omitempty"`
	RevisionID string    `json:"revisionId,omitempty"`
	Kind       string    `json:"kind"`
	ActorID    string    `json:"actorId,omitempty"`
	OccurredAt time.Time `json:"occurredAt"`
}

type OutboxMessage struct {
	ID         string    `json:"id"`
	ContractID string    `json:"contractId"`
	TrackID    string    `json:"trackId,omitempty"`
	RevisionID string    `json:"revisionId,omitempty"`
	Topic      string    `json:"topic"`
	Payload    []byte    `json:"payload,omitempty"`
	CreatedAt  time.Time `json:"createdAt"`
}
