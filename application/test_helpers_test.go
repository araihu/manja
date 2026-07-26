package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/araihu/manja/application/port"
	"github.com/araihu/manja/domain"
)

type testOperationalStore struct {
	revisions    map[string]domain.ContractRevision
	reviews      map[string]domain.ContractReview
	syncRecords  map[string]domain.SyncRecord
	tracks       map[string]domain.ReleaseTrack
	publications map[string]domain.Publication
	auditEvents  []domain.AuditEvent
	outbox       []domain.OutboxMessage
	calls        []string
	contexts     []context.Context
	failAt       string
}

func newTestOperationalStore() *testOperationalStore {
	return &testOperationalStore{
		revisions:    map[string]domain.ContractRevision{},
		reviews:      map[string]domain.ContractReview{},
		syncRecords:  map[string]domain.SyncRecord{},
		tracks:       map[string]domain.ReleaseTrack{},
		publications: map[string]domain.Publication{},
	}
}

func (s *testOperationalStore) clone() *testOperationalStore {
	next := newTestOperationalStore()
	for key, value := range s.revisions {
		next.revisions[key] = value
	}
	for key, value := range s.reviews {
		next.reviews[key] = value
	}
	for key, value := range s.syncRecords {
		next.syncRecords[key] = value
	}
	for key, value := range s.tracks {
		next.tracks[key] = domain.CloneReleaseTrack(value)
	}
	for key, value := range s.publications {
		next.publications[key] = value
	}
	next.auditEvents = append([]domain.AuditEvent(nil), s.auditEvents...)
	next.outbox = append([]domain.OutboxMessage(nil), s.outbox...)
	next.calls = append([]string(nil), s.calls...)
	next.contexts = append([]context.Context(nil), s.contexts...)
	next.failAt = s.failAt
	return next
}

func (s *testOperationalStore) record(ctx context.Context, name string) error {
	s.calls = append(s.calls, name)
	s.contexts = append(s.contexts, ctx)
	if s.failAt == name {
		return errors.New("forced " + name + " failure")
	}
	return nil
}

func (s *testOperationalStore) SaveRevision(ctx context.Context, value domain.ContractRevision) error {
	if err := s.record(ctx, "revision"); err != nil {
		return err
	}
	s.revisions[value.ID] = value
	return nil
}

func (s *testOperationalStore) ContractRevision(ctx context.Context, contractID, revisionID string) (domain.ContractRevision, error) {
	if err := s.record(ctx, "revision-read"); err != nil {
		return domain.ContractRevision{}, err
	}
	revision, ok := s.revisions[revisionID]
	if !ok || revision.ContractID != contractID {
		return domain.ContractRevision{}, errors.New("revision not found")
	}
	return revision, nil
}

func (s *testOperationalStore) SaveReview(ctx context.Context, value domain.ContractReview) error {
	if err := s.record(ctx, "review"); err != nil {
		return err
	}
	s.reviews[value.ID] = value
	return nil
}

func (s *testOperationalStore) SaveSyncRecord(ctx context.Context, value domain.SyncRecord) error {
	if err := s.record(ctx, "sync"); err != nil {
		return err
	}
	s.syncRecords[value.ID] = value
	return nil
}

func (s *testOperationalStore) ReleaseTrack(ctx context.Context, contractID, trackID string) (domain.ReleaseTrack, error) {
	if err := s.record(ctx, "track-read"); err != nil {
		return domain.ReleaseTrack{}, err
	}
	track, ok := s.tracks[contractID+"/"+trackID]
	if !ok {
		return domain.ReleaseTrack{}, errors.New("track not found")
	}
	if err := domain.ValidateReleaseTrack(track); err != nil {
		return domain.ReleaseTrack{}, err
	}
	return domain.CloneReleaseTrack(track), nil
}

func (s *testOperationalStore) SaveReleaseTrack(ctx context.Context, expected uint64, value domain.ReleaseTrack) error {
	if err := s.record(ctx, "track-write"); err != nil {
		return err
	}
	if err := domain.ValidateReleaseTrack(value); err != nil {
		return err
	}
	key := value.ContractID + "/" + value.ID
	current, ok := s.tracks[key]
	if ok && current.Generation != expected || !ok && expected != 0 {
		return port.ErrGenerationConflict
	}
	if ok {
		if err := domain.ValidateReleaseTrackTransition(current, value); err != nil {
			return err
		}
	}
	s.tracks[key] = domain.CloneReleaseTrack(value)
	return nil
}

func (s *testOperationalStore) SavePublication(ctx context.Context, value domain.Publication) error {
	if err := s.record(ctx, "publication"); err != nil {
		return err
	}
	s.publications[value.ProjectID+"/"+value.RevisionID] = value
	return nil
}

func (s *testOperationalStore) AppendAuditEvent(ctx context.Context, value domain.AuditEvent) error {
	if err := s.record(ctx, "audit"); err != nil {
		return err
	}
	s.auditEvents = append(s.auditEvents, value)
	return nil
}

func (s *testOperationalStore) Enqueue(ctx context.Context, value domain.OutboxMessage) error {
	if err := s.record(ctx, "outbox"); err != nil {
		return err
	}
	s.outbox = append(s.outbox, value)
	return nil
}

type testUnitOfWork struct {
	committed  *testOperationalStore
	ctx        context.Context
	failCommit bool
}

func (u *testUnitOfWork) Within(ctx context.Context, callback func(context.Context, port.OperationalStore) error) error {
	u.ctx = ctx
	staged := u.committed.clone()
	if err := callback(ctx, staged); err != nil {
		return err
	}
	if u.failCommit {
		return errors.New("forced commit failure")
	}
	*u.committed = *staged
	return nil
}

type testClock struct {
	now      time.Time
	contexts []context.Context
}

func (c *testClock) Now(ctx context.Context) time.Time {
	c.contexts = append(c.contexts, ctx)
	return c.now
}

func assertSameContexts(t *testing.T, want context.Context, contexts ...context.Context) {
	t.Helper()
	for _, got := range contexts {
		if got != want {
			t.Fatal("port received a replacement context")
		}
	}
}
