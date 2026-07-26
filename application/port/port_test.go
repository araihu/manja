package port

import (
	"context"
	"testing"

	"github.com/araihu/manja/domain"
)

type contextMarker struct{}

func TestUnitOfWorkExposesCompleteOperationalInvariant(t *testing.T) {
	ctx := context.WithValue(context.Background(), contextMarker{}, "transaction")
	store := &recordingOperationalStore{}
	uow := recordingUnitOfWork{store: store}
	err := uow.Within(ctx, func(callbackContext context.Context, operational OperationalStore) error {
		if callbackContext != ctx {
			t.Fatal("unit of work replaced the incoming context")
		}
		operations := []struct {
			name string
			run  func() error
		}{
			{"revision", func() error {
				return operational.SaveRevision(callbackContext, domain.ContractRevision{ID: "revision-1"})
			}},
			{"revision read", func() error {
				_, err := operational.ContractRevision(callbackContext, "payments", "revision-1")
				return err
			}},
			{"review", func() error { return operational.SaveReview(callbackContext, domain.ContractReview{ID: "review-1"}) }},
			{"sync record", func() error { return operational.SaveSyncRecord(callbackContext, domain.SyncRecord{ID: "sync-1"}) }},
			{"release track read", func() error { _, err := operational.ReleaseTrack(callbackContext, "payments", "v1"); return err }},
			{"release track write", func() error {
				return operational.SaveReleaseTrack(callbackContext, 0, domain.ReleaseTrack{ID: "v1", ContractID: "payments"})
			}},
			{"publication", func() error {
				return operational.SavePublication(callbackContext, domain.Publication{ProjectID: "payments", RevisionID: "revision-1"})
			}},
			{"audit", func() error { return operational.AppendAuditEvent(callbackContext, domain.AuditEvent{ID: "audit-1"}) }},
			{"outbox", func() error { return operational.Enqueue(callbackContext, domain.OutboxMessage{ID: "outbox-1"}) }},
		}
		for _, operation := range operations {
			if err := operation.run(); err != nil {
				t.Fatalf("%s: %v", operation.name, err)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("unit of work: %v", err)
	}
	if len(store.calls) != 9 {
		t.Fatalf("operational calls = %v, want all nine invariant participants", store.calls)
	}
	for _, got := range store.contexts {
		if got != ctx {
			t.Fatal("operational store received a replacement context")
		}
	}
}

func TestContentAddressedBlobKeyIsDeterministic(t *testing.T) {
	first := ContentAddressedBlobKey([]byte("openapi: 3.1.0\n"))
	replay := ContentAddressedBlobKey([]byte("openapi: 3.1.0\n"))
	different := ContentAddressedBlobKey([]byte("openapi: 3.0.3\n"))
	if first != replay {
		t.Fatalf("replayed bytes produced keys %q and %q", first, replay)
	}
	if first == different {
		t.Fatalf("different bytes produced the same key %q", first)
	}
	if !first.Valid() {
		t.Fatalf("content-addressed key %q is invalid", first)
	}
}

func TestSecretRefRejectsMissingOrUnnormalizedName(t *testing.T) {
	for _, ref := range []SecretRef{{}, {Name: " vault/payments "}} {
		if err := ref.Validate(); err == nil {
			t.Fatalf("Validate(%q) succeeded, want error", ref.Name)
		}
	}
	if err := (SecretRef{Name: "vault/payments"}).Validate(); err != nil {
		t.Fatalf("validate opaque secret reference: %v", err)
	}
}

type recordingUnitOfWork struct {
	store OperationalStore
}

func (u recordingUnitOfWork) Within(ctx context.Context, callback func(context.Context, OperationalStore) error) error {
	return callback(ctx, u.store)
}

type recordingOperationalStore struct {
	calls    []string
	contexts []context.Context
}

func (s *recordingOperationalStore) record(ctx context.Context, name string) {
	s.contexts = append(s.contexts, ctx)
	s.calls = append(s.calls, name)
}

func (s *recordingOperationalStore) SaveRevision(ctx context.Context, _ domain.ContractRevision) error {
	s.record(ctx, "revision")
	return nil
}

func (s *recordingOperationalStore) ContractRevision(ctx context.Context, _, _ string) (domain.ContractRevision, error) {
	s.record(ctx, "revision-read")
	return domain.ContractRevision{}, nil
}

func (s *recordingOperationalStore) SaveReview(ctx context.Context, _ domain.ContractReview) error {
	s.record(ctx, "review")
	return nil
}

func (s *recordingOperationalStore) SaveSyncRecord(ctx context.Context, _ domain.SyncRecord) error {
	s.record(ctx, "sync")
	return nil
}

func (s *recordingOperationalStore) ReleaseTrack(ctx context.Context, _, _ string) (domain.ReleaseTrack, error) {
	s.record(ctx, "track-read")
	return domain.ReleaseTrack{}, nil
}

func (s *recordingOperationalStore) SaveReleaseTrack(ctx context.Context, _ uint64, _ domain.ReleaseTrack) error {
	s.record(ctx, "track-write")
	return nil
}

func (s *recordingOperationalStore) SavePublication(ctx context.Context, _ domain.Publication) error {
	s.record(ctx, "publication")
	return nil
}

func (s *recordingOperationalStore) AppendAuditEvent(ctx context.Context, _ domain.AuditEvent) error {
	s.record(ctx, "audit")
	return nil
}

func (s *recordingOperationalStore) Enqueue(ctx context.Context, _ domain.OutboxMessage) error {
	s.record(ctx, "outbox")
	return nil
}

var _ UnitOfWork = recordingUnitOfWork{}
var _ OperationalStore = (*recordingOperationalStore)(nil)
