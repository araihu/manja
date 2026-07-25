// Package port defines infrastructure contracts used by reusable Manja
// application services. Implementations live outside application.
package port

import (
	"context"
	"errors"

	"github.com/araihu/manja/domain"
)

var ErrGenerationConflict = errors.New("operational generation conflict")

// UnitOfWork provides one atomic boundary for consistency-sensitive operational
// state. Implementations must roll back every callback mutation when the
// callback or commit fails.
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
