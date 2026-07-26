package extension_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/araihu/manja/application"
	"github.com/araihu/manja/application/port"
	"github.com/araihu/manja/contracttest"
	"github.com/araihu/manja/domain"
)

type contextKey struct{}

func TestUnrelatedModuleExecutesReviewAndSync(t *testing.T) {
	ctx := context.WithValue(context.Background(), contextKey{}, "extension")
	loader := &memoryInputLoader{}
	builder := &memorySnapshotBuilder{}
	check, err := application.NewCheckService(application.CheckDependencies{
		Inputs:    loader,
		Snapshots: builder,
	})
	if err != nil {
		t.Fatalf("construct check service: %v", err)
	}
	policy, err := domain.MergePolicy(domain.PolicyLayer{
		Name:   "repository-default",
		Source: domain.PolicySourceRepository,
	})
	if err != nil {
		t.Fatalf("merge policy: %v", err)
	}
	report, err := check.Run(ctx, application.CheckRequest{
		ContractID:    "payments",
		SpecPath:      "openapi.yaml",
		Target:        domain.ReviewInputLocator{File: "target.yaml"},
		Candidate:     domain.ReviewInputLocator{File: "candidate.yaml"},
		Policy:        policy,
		EvaluatedAt:   time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC),
		EngineVersion: "extension-test",
	})
	if err != nil {
		t.Fatalf("run public review: %v", err)
	}
	if report.Verdict != domain.VerdictPass {
		t.Fatalf("review verdict = %q, want pass", report.Verdict)
	}

	blobs := newMemoryBlobStore()
	operational := newMemoryOperationalStore()
	uow := &memoryUnitOfWork{store: operational, blobs: blobs}
	source := &memorySource{}
	parser := &memoryParser{}
	clock := &fixedClock{now: time.Date(2026, 7, 25, 13, 0, 0, 0, time.UTC)}
	syncer, err := application.NewSyncService(application.SyncDependencies{
		Source:     source,
		Parser:     parser,
		UnitOfWork: uow,
		Blobs:      blobs,
		Clock:      clock,
	})
	if err != nil {
		t.Fatalf("construct sync service: %v", err)
	}
	result, err := syncer.Sync(ctx, application.SyncCommand{
		ContractID: "payments",
		SourceID:   "source-main",
		Trigger:    "extension-test",
	})
	if err != nil {
		t.Fatalf("run public sync: %v", err)
	}
	if result.Record.Result != domain.SyncResultSuccess {
		t.Fatalf("sync result = %q, want success", result.Record.Result)
	}
	for name, got := range map[string]context.Context{
		"review loader":    loader.contexts[0],
		"snapshot builder": builder.contexts[0],
		"sync source":      source.ctx,
		"sync parser":      parser.ctx,
		"blob store":       blobs.ctx,
		"unit of work":     uow.ctx,
		"clock":            clock.ctx,
	} {
		if got != ctx {
			t.Fatalf("%s received replacement context", name)
		}
	}
}

func TestLegacyReleaseReviewValidatorRemainsUsable(t *testing.T) {
	evaluatedAt := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	baseline := domain.NewContractSnapshot("payments", "revision-good", []byte("baseline"), domain.SpecIndex{})
	candidate := domain.NewContractSnapshot("payments", "revision-next", []byte("candidate"), domain.SpecIndex{})
	policy, err := domain.MergePolicy(domain.PolicyLayer{
		Name: "repository-default", Source: domain.PolicySourceRepository,
	})
	if err != nil {
		t.Fatal(err)
	}
	report, err := domain.EvaluateReview(domain.ReviewRequest{
		ContractID: "payments", Target: baseline, Candidate: candidate, Release: &baseline,
		Policy: policy, EvaluatedAt: evaluatedAt, EngineVersion: "extension-test",
	})
	if err != nil {
		t.Fatal(err)
	}
	report.Comparisons = []domain.ComparisonReport{report.Comparisons[1]}
	report.Verdict = report.Comparisons[0].Policy.Verdict

	if err := domain.ValidateReleaseReviewReport(
		report,
		"payments",
		report.Comparisons[0].Baseline,
		report.Comparisons[0].Candidate,
	); err != nil {
		t.Fatalf("legacy release review validation: %v", err)
	}
}

func TestPublicContractSuitesAreUsableByUnrelatedModule(t *testing.T) {
	contracttest.UnitOfWork(t, func(testing.TB) port.UnitOfWork {
		return &memoryUnitOfWork{store: newMemoryOperationalStore(), blobs: newMemoryBlobStore()}
	})
	contracttest.BlobStore(t, func(testing.TB) port.BlobStore {
		return newMemoryBlobStore()
	})
}

type memoryInputLoader struct {
	contexts []context.Context
}

func (l *memoryInputLoader) Load(ctx context.Context, _ string, locator domain.ReviewInputLocator) (domain.SpecFile, domain.ContractRevision, error) {
	l.contexts = append(l.contexts, ctx)
	return domain.SpecFile{Path: locator.File, Bytes: []byte(locator.File)}, domain.ContractRevision{ID: locator.File}, nil
}

type memorySnapshotBuilder struct {
	contexts []context.Context
}

func (b *memorySnapshotBuilder) Build(ctx context.Context, contractID string, file domain.SpecFile, revision domain.ContractRevision) (domain.ContractSnapshot, error) {
	b.contexts = append(b.contexts, ctx)
	return domain.NewContractSnapshot(contractID, revision.ID, file.Bytes, domain.SpecIndex{}), nil
}

type memorySource struct {
	ctx context.Context
}

func (s *memorySource) Fetch(ctx context.Context) (domain.SpecFile, domain.ContractRevision, error) {
	s.ctx = ctx
	return domain.SpecFile{
			SourceID: "source-main",
			Path:     "openapi.yaml",
			Format:   "yaml",
			Bytes:    []byte("openapi: 3.1.0\n"),
		}, domain.ContractRevision{
			ID:       "revision-1",
			SourceID: "source-main",
			Ref:      "main",
		}, nil
}

type memoryParser struct {
	ctx context.Context
}

func (p *memoryParser) Parse(ctx context.Context, _ domain.SpecFile, revision domain.ContractRevision) (domain.SpecIndex, error) {
	p.ctx = ctx
	return domain.SpecIndex{RevisionID: revision.ID, Title: "Payments"}, nil
}

type fixedClock struct {
	now time.Time
	ctx context.Context
}

func (c *fixedClock) Now(ctx context.Context) time.Time {
	c.ctx = ctx
	return c.now
}

type memoryBlobStore struct {
	ctx   context.Context
	blobs map[port.BlobKey][]byte
}

func newMemoryBlobStore() *memoryBlobStore {
	return &memoryBlobStore{blobs: map[port.BlobKey][]byte{}}
}

func (s *memoryBlobStore) Put(ctx context.Context, data []byte) (port.BlobKey, error) {
	s.ctx = ctx
	if err := ctx.Err(); err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	key := port.BlobKey("sha256:" + hex.EncodeToString(sum[:]))
	s.blobs[key] = append([]byte(nil), data...)
	return key, nil
}

func (s *memoryBlobStore) Get(ctx context.Context, key port.BlobKey) ([]byte, error) {
	s.ctx = ctx
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	data, ok := s.blobs[key]
	if !ok {
		return nil, errors.New("blob not found")
	}
	return append([]byte(nil), data...), nil
}

type memoryUnitOfWork struct {
	ctx   context.Context
	mu    sync.Mutex
	store *memoryOperationalStore
	blobs *memoryBlobStore
}

func (u *memoryUnitOfWork) Within(ctx context.Context, fn func(context.Context, port.OperationalStore) error) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.ctx = ctx
	staged := u.store.clone()
	if err := fn(ctx, staged); err != nil {
		return err
	}
	for _, revision := range staged.revisions {
		if revision.SpecBlobKey == "" {
			continue
		}
		if _, ok := u.blobs.blobs[port.BlobKey(revision.SpecBlobKey)]; !ok {
			return errors.New("revision references missing blob")
		}
	}
	if err := validateMemoryOperationalReferences(staged); err != nil {
		return err
	}
	*u.store = *staged
	return nil
}

func validateMemoryOperationalReferences(store *memoryOperationalStore) error {
	requireOwner := func(contractID, revisionID, owner string) error {
		revision, ok := store.revisions[revisionID]
		if !ok {
			return errors.New(owner + " references missing revision")
		}
		if revision.ContractID != contractID {
			return errors.New(owner + " references a revision owned by another contract")
		}
		return nil
	}
	for _, track := range store.tracks {
		if track.CurrentRevisionID != "" {
			if err := requireOwner(track.ContractID, track.CurrentRevisionID, "release track current"); err != nil {
				return err
			}
		}
		if track.CandidateRevisionID != "" {
			if err := requireOwner(track.ContractID, track.CandidateRevisionID, "release track candidate"); err != nil {
				return err
			}
		}
	}
	for _, publication := range store.publications {
		if err := requireOwner(publication.ProjectID, publication.RevisionID, "publication"); err != nil {
			return err
		}
	}
	for _, review := range store.reviews {
		if err := requireOwner(review.ContractID, review.BaselineRevisionID, "review baseline"); err != nil {
			return err
		}
		if err := requireOwner(review.ContractID, review.CandidateRevisionID, "review candidate"); err != nil {
			return err
		}
	}
	for _, record := range store.syncRecords {
		if record.Result == domain.SyncResultSuccess && record.RevisionID != "" {
			if err := requireOwner(record.ProjectID, record.RevisionID, "sync record"); err != nil {
				return err
			}
		}
	}
	for _, event := range store.auditEvents {
		if event.RevisionID != "" {
			if err := requireOwner(event.ContractID, event.RevisionID, "audit event"); err != nil {
				return err
			}
		}
	}
	for _, message := range store.outbox {
		if message.RevisionID != "" {
			if err := requireOwner(message.ContractID, message.RevisionID, "outbox message"); err != nil {
				return err
			}
		}
	}
	return nil
}

type memoryOperationalStore struct {
	revisions    map[string]domain.ContractRevision
	reviews      map[string]domain.ContractReview
	syncRecords  map[string]domain.SyncRecord
	tracks       map[string]domain.ReleaseTrack
	publications map[string]domain.Publication
	auditEvents  []domain.AuditEvent
	outbox       []domain.OutboxMessage
}

func newMemoryOperationalStore() *memoryOperationalStore {
	return &memoryOperationalStore{
		revisions:    map[string]domain.ContractRevision{},
		reviews:      map[string]domain.ContractReview{},
		syncRecords:  map[string]domain.SyncRecord{},
		tracks:       map[string]domain.ReleaseTrack{},
		publications: map[string]domain.Publication{},
	}
}

func (s *memoryOperationalStore) clone() *memoryOperationalStore {
	next := newMemoryOperationalStore()
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
	return next
}

func (s *memoryOperationalStore) SaveRevision(_ context.Context, revision domain.ContractRevision) error {
	s.revisions[revision.ID] = revision
	return nil
}

func (s *memoryOperationalStore) SaveReview(_ context.Context, review domain.ContractReview) error {
	s.reviews[review.ID] = review
	return nil
}

func (s *memoryOperationalStore) SaveSyncRecord(_ context.Context, record domain.SyncRecord) error {
	s.syncRecords[record.ID] = record
	return nil
}

func (s *memoryOperationalStore) ReleaseTrack(_ context.Context, contractID, trackID string) (domain.ReleaseTrack, error) {
	track, ok := s.tracks[contractID+"/"+trackID]
	if !ok {
		return domain.ReleaseTrack{}, errors.New("track not found")
	}
	if err := domain.ValidateReleaseTrack(track); err != nil {
		return domain.ReleaseTrack{}, err
	}
	return domain.CloneReleaseTrack(track), nil
}

func (s *memoryOperationalStore) SaveReleaseTrack(_ context.Context, expectedGeneration uint64, track domain.ReleaseTrack) error {
	if err := domain.ValidateReleaseTrack(track); err != nil {
		return err
	}
	key := track.ContractID + "/" + track.ID
	current, ok := s.tracks[key]
	if ok && current.Generation != expectedGeneration {
		return port.ErrGenerationConflict
	}
	if !ok && expectedGeneration != 0 {
		return port.ErrGenerationConflict
	}
	if ok {
		if err := domain.ValidateReleaseTrackTransition(current, track); err != nil {
			return err
		}
	}
	s.tracks[key] = domain.CloneReleaseTrack(track)
	return nil
}

func (s *memoryOperationalStore) SavePublication(_ context.Context, publication domain.Publication) error {
	s.publications[publication.ProjectID+"/"+publication.RevisionID] = publication
	return nil
}

func (s *memoryOperationalStore) AppendAuditEvent(_ context.Context, event domain.AuditEvent) error {
	s.auditEvents = append(s.auditEvents, event)
	return nil
}

func (s *memoryOperationalStore) Enqueue(_ context.Context, message domain.OutboxMessage) error {
	s.outbox = append(s.outbox, message)
	return nil
}
