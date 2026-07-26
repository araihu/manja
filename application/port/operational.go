// Package port defines infrastructure contracts used by reusable Manja
// application services. Implementations live outside application.
package port

import (
	"context"
	"errors"

	"github.com/araihu/manja/domain"
)

var (
	ErrGenerationConflict   = errors.New("operational generation conflict")
	ErrCommitOutcomeUnknown = errors.New("operational commit outcome unknown")
)

// UnitOfWork provides one atomic boundary for consistency-sensitive operational
// state. Implementations must roll back every callback mutation when the
// callback fails or commit fails before atomic publication. If atomic
// publication succeeds but its durability cannot be confirmed, Within returns
// an error wrapping ErrCommitOutcomeUnknown; callers must reload state and may
// retry only idempotent operations because either the prior or next complete
// state can survive restart.
type UnitOfWork interface {
	Within(context.Context, func(context.Context, OperationalStore) error) error
}

// OperationalStore exposes every record that may participate in revision,
// review, sync, release, publication, audit, and outbox invariants. A production
// implementation must not make partial commits observable.
type OperationalStore interface {
	SaveRevision(context.Context, domain.ContractRevision) error
	SaveReview(context.Context, domain.ContractReview) error
	SaveSyncRecord(context.Context, domain.SyncRecord) error
	ReleaseTrack(context.Context, string, string) (domain.ReleaseTrack, error)
	SaveReleaseTrack(context.Context, uint64, domain.ReleaseTrack) error
	SavePublication(context.Context, domain.Publication) error
	AppendAuditEvent(context.Context, domain.AuditEvent) error
	Enqueue(context.Context, domain.OutboxMessage) error
}

// RevisionReader loads immutable revision evidence by its contract-scoped
// identity without expanding the transactional operational write boundary.
type RevisionReader interface {
	ContractRevision(context.Context, string, string) (domain.ContractRevision, error)
}

// ReleaseEvidenceReader resolves the immutable, track-scoped authorization,
// review, and sync bundle used at the release trust boundary.
type ReleaseEvidenceReader interface {
	ReleaseEvidence(context.Context, string, string, string) (domain.ReleaseEvidence, error)
}

// ReleaseAuthorizationWriter persists a deployment-owned review authorization
// without expanding the eight-method transactional OperationalStore contract.
type ReleaseAuthorizationWriter interface {
	SaveReleaseAuthorization(context.Context, domain.ReleaseAuthorization) error
}

// PublicationReader resolves the self-hosted public route without exposing a
// concrete persistence adapter to the application layer.
type PublicationReader interface {
	PublicPublicationByPath(context.Context, string) (domain.Publication, error)
}
