package contracttest

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/araihu/manja/application/port"
	"github.com/araihu/manja/domain"
)

func TestContractSuitesRejectBrokenAdapters(t *testing.T) {
	for _, test := range []struct {
		scenario string
		want     string
	}{
		{scenario: "partial-commit", want: "rollback"},
		{scenario: "replaced-context", want: "context"},
		{scenario: "missing-blob", want: "missing blob"},
		{scenario: "lost-update", want: "generation"},
		{scenario: "non-idempotent-blob", want: "content-addressed"},
		{scenario: "nondeterministic-discovery", want: "deterministic"},
	} {
		t.Run(test.scenario, func(t *testing.T) {
			command := exec.Command(os.Args[0], "-test.run=^TestBrokenContractAdapter$", "-test.v")
			command.Env = append(os.Environ(), "MANJA_CONTRACTTEST_BROKEN="+test.scenario)
			output, err := command.CombinedOutput()
			if err == nil {
				t.Fatalf("broken adapter %q passed its conformance suite\n%s", test.scenario, output)
			}
			if !strings.Contains(strings.ToLower(string(output)), test.want) {
				t.Fatalf("broken adapter %q failure did not mention %q\n%s", test.scenario, test.want, output)
			}
		})
	}
}

func TestBrokenContractAdapter(t *testing.T) {
	scenario := os.Getenv("MANJA_CONTRACTTEST_BROKEN")
	if scenario == "" {
		t.Skip("subprocess helper")
	}
	switch scenario {
	case "partial-commit", "replaced-context", "missing-blob", "lost-update":
		UnitOfWork(t, func(testing.TB) port.UnitOfWork {
			return newTestUnitOfWork(scenario)
		})
	case "non-idempotent-blob":
		BlobStore(t, func(testing.TB) port.BlobStore { return &brokenBlobStore{} })
	case "nondeterministic-discovery":
		SourceFetcher(t, func(testing.TB) SourceFixture {
			source := &brokenSource{}
			return SourceFixture{
				Fetcher:      source,
				WantSpec:     domain.SpecFile{SourceID: "source", Path: "openapi.yaml", Format: "yaml", Bytes: []byte("openapi: 3.1.0\n")},
				WantRevision: domain.ContractRevision{ID: "revision", SourceID: "source", Ref: "main"},
				WantCandidates: []domain.RevisionCandidate{
					{SourceID: "source", Kind: "branch", Ref: "main", CommitSHA: "one"},
					{SourceID: "source", Kind: "tag", Ref: "v1", CommitSHA: "one"},
				},
			}
		})
	default:
		t.Fatalf("unknown broken scenario %q", scenario)
	}
}

type testUnitOfWork struct {
	scenario string
	mu       sync.Mutex
	state    *testOperationalStore
}

func newTestUnitOfWork(scenario string) *testUnitOfWork {
	return &testUnitOfWork{scenario: scenario, state: newTestOperationalStore()}
}

func (u *testUnitOfWork) Within(ctx context.Context, callback func(context.Context, port.OperationalStore) error) error {
	if u.scenario != "lost-update" {
		u.mu.Lock()
		defer u.mu.Unlock()
	}
	staged := u.state.clone()
	callbackContext := ctx
	if u.scenario == "replaced-context" {
		callbackContext = context.Background()
	}
	err := callback(callbackContext, staged)
	if err != nil && u.scenario != "partial-commit" {
		return err
	}
	if u.scenario != "missing-blob" {
		for _, revision := range staged.revisions {
			if revision.SpecBlobKey != "" {
				return errors.New("missing blob")
			}
		}
	}
	if u.scenario == "lost-update" {
		time.Sleep(5 * time.Millisecond)
		u.mu.Lock()
		defer u.mu.Unlock()
	}
	u.state = staged
	return err
}

type testOperationalStore struct {
	revisions map[string]domain.ContractRevision
	tracks    map[string]domain.ReleaseTrack
}

func newTestOperationalStore() *testOperationalStore {
	return &testOperationalStore{revisions: map[string]domain.ContractRevision{}, tracks: map[string]domain.ReleaseTrack{}}
}

func (s *testOperationalStore) clone() *testOperationalStore {
	next := newTestOperationalStore()
	for key, value := range s.revisions {
		next.revisions[key] = value
	}
	for key, value := range s.tracks {
		next.tracks[key] = value
	}
	return next
}

func (s *testOperationalStore) SaveRevision(_ context.Context, revision domain.ContractRevision) error {
	s.revisions[revision.ID] = revision
	return nil
}
func (s *testOperationalStore) ContractRevision(_ context.Context, contractID, revisionID string) (domain.ContractRevision, error) {
	revision, ok := s.revisions[revisionID]
	if !ok || revision.ContractID != contractID {
		return domain.ContractRevision{}, errors.New("revision not found")
	}
	return revision, nil
}
func (*testOperationalStore) SaveReview(context.Context, domain.ContractReview) error { return nil }
func (*testOperationalStore) SaveSyncRecord(context.Context, domain.SyncRecord) error { return nil }
func (s *testOperationalStore) ReleaseTrack(_ context.Context, contractID, trackID string) (domain.ReleaseTrack, error) {
	track, ok := s.tracks[contractID+"/"+trackID]
	if !ok {
		return domain.ReleaseTrack{}, errors.New("track not found")
	}
	return track, nil
}
func (s *testOperationalStore) SaveReleaseTrack(_ context.Context, expected uint64, track domain.ReleaseTrack) error {
	key := track.ContractID + "/" + track.ID
	current, exists := s.tracks[key]
	if (exists && current.Generation != expected) || (!exists && expected != 0) {
		return port.ErrGenerationConflict
	}
	s.tracks[key] = track
	return nil
}
func (*testOperationalStore) SavePublication(context.Context, domain.Publication) error { return nil }
func (*testOperationalStore) AppendAuditEvent(context.Context, domain.AuditEvent) error { return nil }
func (*testOperationalStore) Enqueue(context.Context, domain.OutboxMessage) error       { return nil }

type brokenBlobStore struct {
	count int
	data  []byte
}

func (s *brokenBlobStore) Put(context.Context, []byte) (port.BlobKey, error) {
	s.count++
	s.data = []byte("different")
	return port.BlobKey(fmt.Sprintf("sha256:%064x", s.count)), nil
}
func (s *brokenBlobStore) Get(context.Context, port.BlobKey) ([]byte, error) {
	return append([]byte(nil), s.data...), nil
}

type brokenSource struct {
	reversed bool
}

func (*brokenSource) Fetch(ctx context.Context) (domain.SpecFile, domain.ContractRevision, error) {
	if err := ctx.Err(); err != nil {
		return domain.SpecFile{}, domain.ContractRevision{}, err
	}
	return domain.SpecFile{SourceID: "source", Path: "openapi.yaml", Format: "yaml", Bytes: []byte("openapi: 3.1.0\n")}, domain.ContractRevision{ID: "revision", SourceID: "source", Ref: "main"}, nil
}
func (s *brokenSource) Discover(context.Context) ([]domain.RevisionCandidate, error) {
	s.reversed = !s.reversed
	candidates := []domain.RevisionCandidate{
		{SourceID: "source", Kind: "branch", Ref: "main", CommitSHA: "one"},
		{SourceID: "source", Kind: "tag", Ref: "v1", CommitSHA: "one"},
	}
	if s.reversed {
		candidates[0], candidates[1] = candidates[1], candidates[0]
	}
	return candidates, nil
}
