package domain

import "time"

const (
	SyncResultSuccess = "success"
	SyncResultFailure = "failure"
)

type SyncRecord struct {
	ID           string    `json:"id"`
	ProjectID    string    `json:"projectId"`
	SourceID     string    `json:"sourceId"`
	RevisionID   string    `json:"revisionId,omitempty"`
	Trigger      string    `json:"trigger"`
	Ref          string    `json:"ref,omitempty"`
	CommitSHA    string    `json:"commitSha,omitempty"`
	SpecPath     string    `json:"specPath,omitempty"`
	Result       string    `json:"result"`
	ErrorSummary string    `json:"errorSummary,omitempty"`
	StartedAt    time.Time `json:"startedAt"`
	FinishedAt   time.Time `json:"finishedAt"`
}

// SameSyncEvidence reports whether two records describe the same logical sync
// result. StartedAt and FinishedAt are first-observation metadata: a replay of
// the same logical evidence retains the times already stored durably.
func SameSyncEvidence(left, right SyncRecord) bool {
	left.StartedAt = time.Time{}
	left.FinishedAt = time.Time{}
	right.StartedAt = time.Time{}
	right.FinishedAt = time.Time{}
	return left == right
}
