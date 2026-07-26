package domain

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

type ReleaseMode string

const (
	ReleaseModePinned    ReleaseMode = "pinned"
	ReleaseModeFollowing ReleaseMode = "following"
)

type ReleaseTrack struct {
	ID                  string                  `json:"id"`
	ContractID          string                  `json:"contractId"`
	BoundRef            string                  `json:"boundRef,omitempty"`
	Mode                ReleaseMode             `json:"mode"`
	Generation          uint64                  `json:"generation"`
	CurrentRevisionID   string                  `json:"currentRevisionId,omitempty"`
	CandidateRevisionID string                  `json:"candidateRevisionId,omitempty"`
	LastDecision        *ReleaseDecision        `json:"lastDecision,omitempty"`
	DecisionHistory     *ReleaseDecisionHistory `json:"decisionHistory,omitempty"`
}

// ReleaseDecisionHistory records every applied evidence identity so exact
// historical replays remain no-ops after later decisions and restarts.
type ReleaseDecisionHistory struct {
	SeenDecisionIDs []string `json:"seenDecisionIds"`
}

type ReleaseDecision struct {
	RevisionID   string    `json:"revisionId"`
	ReviewID     string    `json:"reviewId"`
	ReviewDigest string    `json:"reviewDigest"`
	Verdict      string    `json:"verdict"`
	Accepted     bool      `json:"accepted"`
	EvaluatedAt  time.Time `json:"evaluatedAt,omitempty"`
}

// ConsiderReleaseRevision applies a completed policy decision to one track.
// Rejected revisions remain visible as candidates but never replace public
// last-known-good state.
func ConsiderReleaseRevision(track ReleaseTrack, revisionID string, accepted bool) (ReleaseTrack, error) {
	if err := validateReleaseTrack(track); err != nil {
		return ReleaseTrack{}, err
	}
	revisionID = strings.TrimSpace(revisionID)
	if revisionID == "" {
		return ReleaseTrack{}, fmt.Errorf("revision id is required")
	}

	verdict := VerdictFail
	if accepted {
		verdict = VerdictPass
	}
	evaluatedAt := time.Unix(0, 0).UTC()
	if track.LastDecision != nil && !track.LastDecision.EvaluatedAt.IsZero() {
		evaluatedAt = track.LastDecision.EvaluatedAt.Add(time.Nanosecond)
	}
	next, _, err := ConsiderReleaseDecision(track, ReleaseDecision{
		RevisionID:   revisionID,
		ReviewID:     "legacy-release-decision",
		ReviewDigest: sha256Hex([]byte(fmt.Sprintf("%s\x00%t", revisionID, accepted))),
		Verdict:      verdict,
		Accepted:     accepted,
		EvaluatedAt:  evaluatedAt,
	})
	return next, err
}

// ConsiderReleaseDecision applies a reviewed decision to one track. Replay
// identity includes the review bytes, verdict, and acceptance, while evaluation
// time orders previously unseen decisions for the same track.
func ConsiderReleaseDecision(track ReleaseTrack, decision ReleaseDecision) (ReleaseTrack, bool, error) {
	if err := validateReleaseTrack(track); err != nil {
		return ReleaseTrack{}, false, err
	}
	if err := validateReleaseDecision(decision); err != nil {
		return ReleaseTrack{}, false, err
	}
	decisionID := releaseDecisionID(decision)
	if seenReleaseDecision(releaseDecisionHistory(track), decisionID) ||
		(track.LastDecision != nil && *track.LastDecision == decision) {
		return track, false, nil
	}
	if decision.EvaluatedAt.IsZero() {
		return ReleaseTrack{}, false, fmt.Errorf("release decision evaluation time is required")
	}
	if track.LastDecision != nil && !track.LastDecision.EvaluatedAt.IsZero() {
		switch {
		case decision.EvaluatedAt.Before(track.LastDecision.EvaluatedAt):
			return ReleaseTrack{}, false, fmt.Errorf("release decision predates the latest applied decision")
		case decision.EvaluatedAt.Equal(track.LastDecision.EvaluatedAt):
			return ReleaseTrack{}, false, fmt.Errorf("different release decisions have the same evaluation time")
		}
	}

	next := track
	next.DecisionHistory = &ReleaseDecisionHistory{
		SeenDecisionIDs: append([]string(nil), releaseDecisionHistory(track)...),
	}
	if track.LastDecision != nil {
		lastDecisionID := releaseDecisionID(*track.LastDecision)
		if !seenReleaseDecision(next.DecisionHistory.SeenDecisionIDs, lastDecisionID) {
			next.DecisionHistory.SeenDecisionIDs = append(next.DecisionHistory.SeenDecisionIDs, lastDecisionID)
		}
	}
	next.DecisionHistory.SeenDecisionIDs = append(next.DecisionHistory.SeenDecisionIDs, decisionID)
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
	if err := validateReleaseTrack(track); err != nil {
		return ReleaseTrack{}, err
	}
	if track.Mode != ReleaseModePinned {
		return ReleaseTrack{}, fmt.Errorf("only pinned tracks require promotion")
	}
	revisionID = strings.TrimSpace(revisionID)
	if revisionID == "" {
		return ReleaseTrack{}, fmt.Errorf("revision id is required")
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
	next := track
	next.Generation++
	next.CurrentRevisionID = revisionID
	next.CandidateRevisionID = ""
	return next, nil
}

func validateReleaseTrack(track ReleaseTrack) error {
	if strings.TrimSpace(track.ID) == "" {
		return fmt.Errorf("release track id is required")
	}
	if strings.TrimSpace(track.ContractID) == "" {
		return fmt.Errorf("release track contract id is required")
	}
	if track.Mode != ReleaseModePinned && track.Mode != ReleaseModeFollowing {
		return fmt.Errorf("unsupported release track mode %q", track.Mode)
	}
	if track.LastDecision != nil {
		if err := validateReleaseDecision(*track.LastDecision); err != nil {
			return fmt.Errorf("invalid last release decision: %w", err)
		}
	}
	history := releaseDecisionHistory(track)
	seen := make(map[string]struct{}, len(history))
	for _, decisionID := range history {
		if !isLowerSHA256(decisionID) {
			return fmt.Errorf("seen release decision id must be lowercase SHA-256")
		}
		if _, duplicate := seen[decisionID]; duplicate {
			return fmt.Errorf("seen release decision id %q is duplicated", decisionID)
		}
		seen[decisionID] = struct{}{}
	}
	if track.LastDecision == nil && len(history) > 0 {
		return fmt.Errorf("seen release decisions require a last release decision")
	}
	if track.LastDecision != nil && len(history) > 0 {
		if !seenReleaseDecision(history, releaseDecisionID(*track.LastDecision)) {
			return fmt.Errorf("last release decision is absent from seen decision history")
		}
	}
	if track.DecisionHistory != nil && len(history) == 0 {
		return fmt.Errorf("release decision history must not be empty")
	}
	return nil
}

func validateReleaseDecision(decision ReleaseDecision) error {
	if strings.TrimSpace(decision.RevisionID) == "" {
		return fmt.Errorf("release decision revision id is required")
	}
	if strings.TrimSpace(decision.ReviewID) == "" {
		return fmt.Errorf("release decision review id is required")
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

func releaseDecisionID(decision ReleaseDecision) string {
	var identity strings.Builder
	for _, value := range []string{
		decision.RevisionID,
		decision.ReviewID,
		decision.ReviewDigest,
		decision.Verdict,
	} {
		identity.WriteString(strconv.Itoa(len(value)))
		identity.WriteByte(':')
		identity.WriteString(value)
	}
	if decision.Accepted {
		identity.WriteByte('1')
	} else {
		identity.WriteByte('0')
	}
	return sha256Hex([]byte(identity.String()))
}

func seenReleaseDecision(seen []string, decisionID string) bool {
	for _, existing := range seen {
		if existing == decisionID {
			return true
		}
	}
	return false
}

func releaseDecisionHistory(track ReleaseTrack) []string {
	if track.DecisionHistory == nil {
		return nil
	}
	return track.DecisionHistory.SeenDecisionIDs
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
