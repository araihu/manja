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
