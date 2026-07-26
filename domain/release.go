package domain

import (
	"fmt"
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
	RevisionID   string `json:"revisionId"`
	ReviewID     string `json:"reviewId"`
	ReviewDigest string `json:"reviewDigest"`
	Verdict      string `json:"verdict"`
	Accepted     bool   `json:"accepted"`
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
	next, _, err := ConsiderReleaseDecision(track, ReleaseDecision{
		RevisionID:   revisionID,
		ReviewID:     "legacy-release-decision",
		ReviewDigest: sha256Hex([]byte(fmt.Sprintf("%s\x00%t", revisionID, accepted))),
		Verdict:      verdict,
		Accepted:     accepted,
	})
	return next, err
}

// ConsiderReleaseDecision applies a reviewed decision to one track. Replay
// identity includes the review bytes and verdict, so a later decision for the
// same revision is not confused with an exact replay.
func ConsiderReleaseDecision(track ReleaseTrack, decision ReleaseDecision) (ReleaseTrack, bool, error) {
	if err := validateReleaseTrack(track); err != nil {
		return ReleaseTrack{}, false, err
	}
	if err := validateReleaseDecision(decision); err != nil {
		return ReleaseTrack{}, false, err
	}
	if track.LastDecision != nil && *track.LastDecision == decision {
		return track, false, nil
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
