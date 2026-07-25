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
	ID                  string      `json:"id"`
	ContractID          string      `json:"contractId"`
	BoundRef            string      `json:"boundRef,omitempty"`
	Mode                ReleaseMode `json:"mode"`
	Generation          uint64      `json:"generation"`
	CurrentRevisionID   string      `json:"currentRevisionId,omitempty"`
	CandidateRevisionID string      `json:"candidateRevisionId,omitempty"`
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

	next := track
	if accepted && track.Mode == ReleaseModeFollowing && track.CurrentRevisionID == revisionID && track.CandidateRevisionID == "" {
		return next, nil
	}
	if track.CandidateRevisionID == revisionID && (!accepted || track.Mode == ReleaseModePinned) {
		return next, nil
	}
	next.Generation++
	if !accepted || track.Mode == ReleaseModePinned {
		next.CandidateRevisionID = revisionID
		return next, nil
	}
	next.CurrentRevisionID = revisionID
	next.CandidateRevisionID = ""
	return next, nil
}

// PromoteReleaseRevision explicitly advances a pinned track to its recorded
// candidate. It cannot promote a different revision or a following track.
func PromoteReleaseRevision(track ReleaseTrack, revisionID string) (ReleaseTrack, error) {
	if err := validateReleaseTrack(track); err != nil {
		return ReleaseTrack{}, err
	}
	if track.Mode != ReleaseModePinned {
		return ReleaseTrack{}, fmt.Errorf("only pinned tracks require promotion")
	}
	revisionID = strings.TrimSpace(revisionID)
	if revisionID == "" || revisionID != track.CandidateRevisionID {
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
