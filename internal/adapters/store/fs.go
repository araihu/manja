package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"

	"github.com/araihu/manja/application/port"
	"github.com/araihu/manja/domain"
)

var errUnsafeStorePath = errors.New("unsafe store path")

const (
	legacyOperationalStateVersion = 1
	operationalStateVersion       = 2
	releaseTrackAuthorityVersion  = 1
)

type releaseTrackAuthority struct {
	Version         int                    `json:"version"`
	Generation      uint64                 `json:"generation"`
	DecisionPresent bool                   `json:"decisionPresent"`
	Decision        domain.ReleaseDecision `json:"decision"`
}

type FileStore struct {
	root          string
	mu            sync.Mutex
	openDirectory func(string) (directorySyncer, error)
}

type directorySyncer interface {
	Sync() error
	Close() error
}

type operationalState struct {
	Version                 int                                `json:"version"`
	Revisions               map[string]domain.ContractRevision `json:"revisions"`
	Reviews                 map[string]domain.ContractReview   `json:"reviews"`
	SyncRecords             map[string]domain.SyncRecord       `json:"syncRecords"`
	ReleaseTracks           map[string]domain.ReleaseTrack     `json:"releaseTracks"`
	ReleaseTrackAuthorities map[string]releaseTrackAuthority   `json:"releaseTrackAuthorities"`
	Publications            map[string]domain.Publication      `json:"publications"`
	AuditEvents             map[string]domain.AuditEvent       `json:"auditEvents"`
	Outbox                  map[string]domain.OutboxMessage    `json:"outbox"`
	migratedRevisions       map[string]struct{}
}

func NewFileStore(root string) *FileStore {
	store := &FileStore{
		root: root,
		openDirectory: func(path string) (directorySyncer, error) {
			return os.Open(path)
		},
	}
	store.discardIncompleteStaging()
	return store
}

func (s *FileStore) Within(ctx context.Context, callback func(context.Context, port.OperationalStore) error) error {
	if callback == nil {
		return fmt.Errorf("operational callback is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	state, err := s.loadOperationalState(ctx)
	if err != nil {
		return err
	}
	transaction := &operationalTransaction{
		state:                  cloneOperationalState(state),
		validatedReleaseTracks: cloneReleaseTracks(state.ReleaseTracks),
		mutatedRevisions:       cloneIDSet(state.migratedRevisions),
	}
	if err := callback(ctx, transaction); err != nil {
		return err
	}
	if err := s.validateOperationalState(ctx, transaction.state, transaction.mutatedRevisions); err != nil {
		return err
	}
	if err := validateCommittedReleaseTracks(transaction.validatedReleaseTracks, transaction.state.ReleaseTracks); err != nil {
		return err
	}
	return s.publishOperationalState(ctx, transaction.state)
}

func (s *FileStore) SaveProject(ctx context.Context, project domain.Project) error {
	if err := validateID(project.ID); err != nil {
		return err
	}
	return s.writeJSON(ctx, "projects", project.ID+".json", project)
}

func (s *FileStore) Project(ctx context.Context, id string) (domain.Project, error) {
	var project domain.Project
	if err := validateID(id); err != nil {
		return project, err
	}
	err := s.readJSON(ctx, "projects", id+".json", &project)
	return project, err
}

func (s *FileStore) SaveRevision(ctx context.Context, revision domain.ContractRevision) error {
	return s.Within(ctx, func(ctx context.Context, operational port.OperationalStore) error {
		return operational.SaveRevision(ctx, revision)
	})
}

func (s *FileStore) Revision(ctx context.Context, id string) (domain.ContractRevision, error) {
	var revision domain.ContractRevision
	if err := validateID(id); err != nil {
		return revision, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.loadOperationalState(ctx)
	if err != nil {
		return revision, err
	}
	if revision, ok := state.Revisions[id]; ok {
		return revision, nil
	}
	err = s.readJSON(ctx, "revisions", id+".json", &revision)
	return revision, err
}

func (s *FileStore) ContractRevision(ctx context.Context, contractID, revisionID string) (domain.ContractRevision, error) {
	if err := validateID(contractID); err != nil {
		return domain.ContractRevision{}, err
	}
	if err := validateID(revisionID); err != nil {
		return domain.ContractRevision{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.loadOperationalState(ctx)
	if err != nil {
		return domain.ContractRevision{}, err
	}
	revision, ok := state.Revisions[revisionID]
	if !ok {
		return domain.ContractRevision{}, fs.ErrNotExist
	}
	if revision.ContractID != contractID {
		return domain.ContractRevision{}, fmt.Errorf("revision %q belongs to contract %q, not %q", revisionID, revision.ContractID, contractID)
	}
	if err := s.validateRevisionEvidence(ctx, revision); err != nil {
		return domain.ContractRevision{}, fmt.Errorf("revision %q has invalid persisted evidence: %w", revisionID, err)
	}
	return revision, nil
}

func (s *FileStore) SavePublication(ctx context.Context, publication domain.Publication) error {
	return s.Within(ctx, func(ctx context.Context, operational port.OperationalStore) error {
		return operational.SavePublication(ctx, publication)
	})
}

func (s *FileStore) Publication(ctx context.Context, projectID, revisionID string) (domain.Publication, error) {
	var publication domain.Publication
	if err := validateID(projectID); err != nil {
		return publication, err
	}
	if err := validateID(revisionID); err != nil {
		return publication, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.loadOperationalState(ctx)
	if err != nil {
		return publication, err
	}
	if publication, ok := state.Publications[publicationKey(projectID, revisionID)]; ok {
		return publication, nil
	}
	err = s.readJSON(ctx, "publications", projectID+"-"+revisionID+".json", &publication)
	return publication, err
}

func (s *FileStore) PublicPublicationByPath(ctx context.Context, publicPath string) (domain.Publication, error) {
	var publication domain.Publication
	if err := validatePublicPath(publicPath); err != nil {
		return publication, err
	}
	if err := ctx.Err(); err != nil {
		return publication, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.loadOperationalState(ctx)
	if err != nil {
		return publication, err
	}
	for _, candidate := range state.Publications {
		if candidate.Public && candidate.Path == publicPath {
			return candidate, nil
		}
	}

	dir := filepath.Join(s.root, "publications")
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return publication, fs.ErrNotExist
	}
	if err != nil {
		return publication, err
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		var candidate domain.Publication
		if err := s.readJSON(ctx, "publications", entry.Name(), &candidate); err != nil {
			return publication, err
		}
		if candidate.Public && candidate.Path == publicPath {
			return candidate, nil
		}
	}
	return publication, fs.ErrNotExist
}

func (s *FileStore) SaveSyncRecord(ctx context.Context, record domain.SyncRecord) error {
	return s.Within(ctx, func(ctx context.Context, operational port.OperationalStore) error {
		return operational.SaveSyncRecord(ctx, record)
	})
}

func (s *FileStore) SyncRecord(ctx context.Context, id string) (domain.SyncRecord, error) {
	var record domain.SyncRecord
	if err := validateID(id); err != nil {
		return record, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.loadOperationalState(ctx)
	if err != nil {
		return record, err
	}
	if record, ok := state.SyncRecords[id]; ok {
		return record, nil
	}
	err = s.readJSON(ctx, "sync-history", id+".json", &record)
	return record, err
}

func (s *FileStore) ReleaseTrack(ctx context.Context, contractID, trackID string) (domain.ReleaseTrack, error) {
	var track domain.ReleaseTrack
	if err := validateID(contractID); err != nil {
		return track, err
	}
	if err := validateID(trackID); err != nil {
		return track, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.loadOperationalState(ctx)
	if err != nil {
		return track, err
	}
	track, ok := state.ReleaseTracks[releaseTrackKey(contractID, trackID)]
	if !ok {
		return domain.ReleaseTrack{}, fs.ErrNotExist
	}
	if err := domain.ValidateReleaseTrack(track); err != nil {
		return domain.ReleaseTrack{}, fmt.Errorf("invalid persisted release track: %w", err)
	}
	return domain.CloneReleaseTrack(track), nil
}

func (s *FileStore) Put(ctx context.Context, data []byte) (port.BlobKey, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	key := port.ContentAddressedBlobKey(data)
	path, err := s.blobPath(key)
	if err != nil {
		return "", err
	}
	if existing, err := os.ReadFile(path); err == nil {
		if port.ContentAddressedBlobKey(existing) != key {
			return "", fmt.Errorf("stored blob %q failed its content identity", key)
		}
		return key, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := writeFileAtomically(path, data, 0o600); err != nil {
		return "", err
	}
	return key, nil
}

func (s *FileStore) Get(ctx context.Context, key port.BlobKey) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	path, err := s.blobPath(key)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}

// GetLegacy supports self-hosted data written before content-addressed blobs.
// New reusable application services never call it.
func (s *FileStore) GetLegacy(ctx context.Context, key string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	path, err := s.safeNamespacePath("blobs", key)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}

func (s *FileStore) loadOperationalState(ctx context.Context) (operationalState, error) {
	state := newOperationalState()
	if err := ctx.Err(); err != nil {
		return state, err
	}
	path := filepath.Join(s.root, "operational", "state.json")
	data, err := os.ReadFile(path)
	migrateLegacy := errors.Is(err, fs.ErrNotExist)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return state, err
	}
	if err == nil {
		if err := json.Unmarshal(data, &state); err != nil {
			return operationalState{}, fmt.Errorf("decode operational state: %w", err)
		}
		state.initializeMaps()
		switch state.Version {
		case legacyOperationalStateVersion:
			if err := migrateOperationalStateV1(&state); err != nil {
				return operationalState{}, err
			}
			if err := validateReleaseTrackAuthorities(state); err != nil {
				return operationalState{}, err
			}
			if err := validateOperationalReferences(state); err != nil {
				return operationalState{}, err
			}
			if err := s.publishOperationalState(ctx, state); err != nil {
				return operationalState{}, fmt.Errorf("persist operational state v2 migration: %w", err)
			}
		case operationalStateVersion:
			if err := validateReleaseTrackAuthorities(state); err != nil {
				return operationalState{}, err
			}
			if err := validateOperationalReferences(state); err != nil {
				return operationalState{}, err
			}
		default:
			return operationalState{}, fmt.Errorf("unsupported operational state version %d", state.Version)
		}
	}
	state.initializeMaps()
	if !migrateLegacy {
		return state, nil
	}
	if err := mergeLegacyJSON(ctx, s, "revisions", state.Revisions, func(revision domain.ContractRevision) string {
		return revision.ID
	}); err != nil {
		return operationalState{}, err
	}
	for id := range state.Revisions {
		state.migratedRevisions[id] = struct{}{}
	}
	if err := mergeLegacyJSON(ctx, s, "publications", state.Publications, func(publication domain.Publication) string {
		return publicationKey(publication.ProjectID, publication.RevisionID)
	}); err != nil {
		return operationalState{}, err
	}
	if err := mergeLegacyJSON(ctx, s, "sync-history", state.SyncRecords, func(record domain.SyncRecord) string {
		return record.ID
	}); err != nil {
		return operationalState{}, err
	}
	if len(state.Revisions) == 0 && len(state.Publications) == 0 && len(state.SyncRecords) == 0 {
		return state, nil
	}
	if err := bindLegacyOperationalRevisionOwners(&state); err != nil {
		return operationalState{}, fmt.Errorf("migrate legacy revision ownership: %w", err)
	}
	if err := validateReleaseTrackAuthorities(state); err != nil {
		return operationalState{}, fmt.Errorf("validate legacy release authority migration: %w", err)
	}
	if err := validateOperationalReferences(state); err != nil {
		return operationalState{}, fmt.Errorf("validate legacy operational references: %w", err)
	}
	revisionIDs := make([]string, 0, len(state.Revisions))
	for revisionID := range state.Revisions {
		revisionIDs = append(revisionIDs, revisionID)
	}
	sort.Strings(revisionIDs)
	awaitingEvidenceEnrichment := false
	for _, revisionID := range revisionIDs {
		revision := state.Revisions[revisionID]
		if err := s.validateRevisionEvidence(ctx, revision); err != nil {
			if legacyRevisionAwaitsEvidenceEnrichment(revision) {
				awaitingEvidenceEnrichment = true
				continue
			}
			return operationalState{}, fmt.Errorf("validate legacy revision %q migration: %w", revisionID, err)
		}
	}
	if awaitingEvidenceEnrichment {
		// A flat pre-snapshot record may be completed by SaveRevision in this
		// transaction. It cannot be published as v2 until that enrichment has
		// passed the normal final commit validation.
		return state, nil
	}
	if err := s.publishOperationalState(ctx, state); err != nil {
		return operationalState{}, fmt.Errorf("persist legacy operational state v2 migration: %w", err)
	}
	return state, nil
}

func legacyRevisionAwaitsEvidenceEnrichment(revision domain.ContractRevision) bool {
	return revision.ReviewSnapshot == nil &&
		revision.SpecBlobKey != "" &&
		(revision.SpecDigest != "" || revision.ContractDigest != "")
}

func mergeLegacyJSON[T any](
	ctx context.Context,
	store *FileStore,
	namespace string,
	target map[string]T,
	key func(T) string,
) error {
	directory := filepath.Join(store.root, namespace)
	entries, err := os.ReadDir(directory)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		var value T
		if err := store.readJSON(ctx, namespace, entry.Name(), &value); err != nil {
			return fmt.Errorf("read legacy %s record %q: %w", namespace, entry.Name(), err)
		}
		valueKey := key(value)
		if _, exists := target[valueKey]; !exists {
			target[valueKey] = value
		}
	}
	return nil
}

func (s *FileStore) publishOperationalState(ctx context.Context, state operationalState) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	dir := filepath.Join(s.root, "operational")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	temporary, err := os.CreateTemp(dir, ".state-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	removeTemporary := true
	defer func() {
		_ = temporary.Close()
		if removeTemporary {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, filepath.Join(dir, "state.json")); err != nil {
		return err
	}
	removeTemporary = false
	directory, err := s.openDirectory(dir)
	if err != nil {
		return fmt.Errorf("%w: open operational state directory: %w", port.ErrCommitOutcomeUnknown, err)
	}
	defer directory.Close()
	if err := directory.Sync(); err != nil {
		return fmt.Errorf("%w: sync operational state directory: %w", port.ErrCommitOutcomeUnknown, err)
	}
	return nil
}

func (s *FileStore) validateOperationalState(ctx context.Context, state operationalState, mutatedRevisions map[string]struct{}) error {
	if state.Version != operationalStateVersion {
		return fmt.Errorf("operational state version %d is not current", state.Version)
	}
	if err := validateReleaseTrackAuthorities(state); err != nil {
		return err
	}
	if err := validateOperationalReferences(state); err != nil {
		return err
	}
	for id := range mutatedRevisions {
		revision := state.Revisions[id]
		if err := s.validateRevisionEvidence(ctx, revision); err != nil {
			return fmt.Errorf("revision %q has invalid persisted evidence: %w", id, err)
		}
	}
	return nil
}

func (s *FileStore) validateRevisionEvidence(ctx context.Context, revision domain.ContractRevision) error {
	hasReviewEvidence := revision.SpecDigest != "" ||
		revision.ContractDigest != "" ||
		revision.ReviewSnapshot != nil
	if revision.SpecBlobKey == "" {
		if hasReviewEvidence {
			return fmt.Errorf("review evidence requires a spec blob")
		}
		return nil
	}

	key := port.BlobKey(revision.SpecBlobKey)
	if !key.Valid() {
		return fmt.Errorf("invalid blob key %q", key)
	}
	blobPath, err := s.blobPath(key)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(blobPath)
	if err != nil {
		return fmt.Errorf("references missing blob %q: %w", key, err)
	}
	contentKey := port.ContentAddressedBlobKey(data)
	if contentKey != key {
		return fmt.Errorf("references corrupt blob %q", key)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if !hasReviewEvidence {
		return nil
	}
	if strings.TrimSpace(revision.ContractID) == "" ||
		revision.SpecDigest == "" ||
		revision.ContractDigest == "" ||
		revision.ReviewSnapshot == nil {
		return fmt.Errorf("review evidence is incomplete")
	}
	if err := domain.ValidateContractSnapshot(*revision.ReviewSnapshot); err != nil {
		return fmt.Errorf("canonical review snapshot: %w", err)
	}
	if revision.ReviewSnapshot.ContractID != revision.ContractID ||
		revision.ReviewSnapshot.RevisionID != revision.ID {
		return fmt.Errorf("canonical review snapshot identity does not match revision")
	}
	if revision.ReviewSnapshot.SpecDigest != revision.SpecDigest {
		return fmt.Errorf("spec digest does not match canonical review snapshot")
	}
	if revision.ReviewSnapshot.ContractDigest != revision.ContractDigest {
		return fmt.Errorf("contract digest does not match canonical review snapshot")
	}
	contentDigest := strings.TrimPrefix(string(contentKey), "sha256:")
	if revision.SpecDigest != contentDigest {
		return fmt.Errorf("spec digest does not match stored blob")
	}
	return nil
}

func (s *FileStore) discardIncompleteStaging() {
	matches, err := filepath.Glob(filepath.Join(s.root, "operational", ".state-*.tmp"))
	if err == nil {
		for _, match := range matches {
			_ = os.Remove(match)
		}
	}
	for _, namespace := range []string{"blobs", "operational", "projects", "publications", "revisions", "sync-history"} {
		_ = filepath.WalkDir(filepath.Join(s.root, namespace), func(path string, entry fs.DirEntry, err error) error {
			if err != nil || entry.IsDir() {
				return nil
			}
			name := entry.Name()
			if strings.HasPrefix(name, ".write-") && strings.HasSuffix(name, ".tmp") {
				_ = os.Remove(path)
			}
			return nil
		})
	}
}

func (s *FileStore) blobPath(key port.BlobKey) (string, error) {
	if !key.Valid() {
		return "", errUnsafeStorePath
	}
	digest := strings.TrimPrefix(string(key), "sha256:")
	return s.safeNamespacePath("blobs", filepath.Join("sha256", digest))
}

func (s *FileStore) writeJSON(ctx context.Context, namespace, name string, value any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	path, err := s.safeNamespacePath(namespace, name)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return writeFileAtomically(path, data, 0o600)
}

func (s *FileStore) readJSON(ctx context.Context, namespace, name string, value any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	path, err := s.safeNamespacePath(namespace, name)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, value)
}

func (s *FileStore) safeNamespacePath(namespace, name string) (string, error) {
	clean := filepath.Clean(name)
	if clean != name || clean == "." || clean == ".." || filepath.IsAbs(clean) || strings.Contains(name, `\`) {
		return "", errUnsafeStorePath
	}
	root := filepath.Join(s.root, namespace)
	fullPath := filepath.Join(root, clean)
	relative, err := filepath.Rel(root, fullPath)
	if err != nil {
		return "", err
	}
	if relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", errUnsafeStorePath
	}
	return fullPath, nil
}

type operationalTransaction struct {
	state                  operationalState
	validatedReleaseTracks map[string]domain.ReleaseTrack
	mutatedRevisions       map[string]struct{}
}

func (t *operationalTransaction) SaveRevision(ctx context.Context, revision domain.ContractRevision) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateID(revision.ID); err != nil {
		return err
	}
	if existing, ok := t.state.Revisions[revision.ID]; ok {
		if !reflect.DeepEqual(existing, revision) {
			if !canEnrichLegacyRevisionEvidence(existing, revision) {
				return fmt.Errorf("revision %q conflicts with immutable persisted evidence", revision.ID)
			}
			t.state.Revisions[revision.ID] = revision
			t.mutatedRevisions[revision.ID] = struct{}{}
		}
		return nil
	}
	t.state.Revisions[revision.ID] = revision
	t.mutatedRevisions[revision.ID] = struct{}{}
	return nil
}

func canEnrichLegacyRevisionEvidence(existing, enriched domain.ContractRevision) bool {
	if existing.ReviewSnapshot != nil || enriched.ReviewSnapshot == nil {
		return false
	}
	existingMetadata := existing
	existingMetadata.SpecDigest = ""
	existingMetadata.ContractDigest = ""
	existingMetadata.ReviewSnapshot = nil

	enrichedMetadata := enriched
	enrichedMetadata.SpecDigest = ""
	enrichedMetadata.ContractDigest = ""
	enrichedMetadata.ReviewSnapshot = nil
	if existing.ContractID == "" {
		enrichedMetadata.ContractID = ""
	}
	return reflect.DeepEqual(existingMetadata, enrichedMetadata)
}

func (t *operationalTransaction) SaveReview(ctx context.Context, review domain.ContractReview) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateID(review.ID); err != nil {
		return err
	}
	if existing, ok := t.state.Reviews[review.ID]; ok {
		if !reflect.DeepEqual(existing, review) {
			return fmt.Errorf("review %q conflicts with immutable persisted evidence", review.ID)
		}
		return nil
	}
	if err := t.bindRevisionOwner(review.ContractID, review.BaselineRevisionID, "review "+review.ID+" baseline"); err != nil {
		return err
	}
	if err := t.bindRevisionOwner(review.ContractID, review.CandidateRevisionID, "review "+review.ID+" candidate"); err != nil {
		return err
	}
	t.state.Reviews[review.ID] = review
	return nil
}

func (t *operationalTransaction) SaveSyncRecord(ctx context.Context, record domain.SyncRecord) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateID(record.ID); err != nil {
		return err
	}
	if record.Result == domain.SyncResultSuccess && record.RevisionID != "" {
		if err := t.bindRevisionOwner(record.ProjectID, record.RevisionID, "sync record "+record.ID); err != nil {
			return err
		}
	}
	t.state.SyncRecords[record.ID] = record
	return nil
}

func (t *operationalTransaction) ReleaseTrack(ctx context.Context, contractID, trackID string) (domain.ReleaseTrack, error) {
	if err := ctx.Err(); err != nil {
		return domain.ReleaseTrack{}, err
	}
	if err := validateID(contractID); err != nil {
		return domain.ReleaseTrack{}, err
	}
	if err := validateID(trackID); err != nil {
		return domain.ReleaseTrack{}, err
	}
	track, ok := t.state.ReleaseTracks[releaseTrackKey(contractID, trackID)]
	if !ok {
		return domain.ReleaseTrack{}, fs.ErrNotExist
	}
	if err := domain.ValidateReleaseTrack(track); err != nil {
		return domain.ReleaseTrack{}, fmt.Errorf("invalid persisted release track: %w", err)
	}
	return domain.CloneReleaseTrack(track), nil
}

func (t *operationalTransaction) SaveReleaseTrack(ctx context.Context, expectedGeneration uint64, track domain.ReleaseTrack) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateID(track.ContractID); err != nil {
		return err
	}
	if err := validateID(track.ID); err != nil {
		return err
	}
	if err := domain.ValidateReleaseTrack(track); err != nil {
		return fmt.Errorf("invalid release track: %w", err)
	}
	key := releaseTrackKey(track.ContractID, track.ID)
	current, exists := t.state.ReleaseTracks[key]
	if exists {
		if err := domain.ValidateReleaseTrack(current); err != nil {
			return fmt.Errorf("invalid persisted release track: %w", err)
		}
	}
	if exists && current.Generation != expectedGeneration {
		return port.ErrGenerationConflict
	}
	if !exists && expectedGeneration != 0 {
		return port.ErrGenerationConflict
	}
	if exists {
		if err := domain.ValidateReleaseTrackTransition(current, track); err != nil {
			return err
		}
	} else if err := validateReleaseTrackInitialization(track); err != nil {
		return err
	}
	if track.CurrentRevisionID != "" {
		if err := t.bindRevisionOwner(track.ContractID, track.CurrentRevisionID, "release track "+key+" current"); err != nil {
			return err
		}
	}
	if track.CandidateRevisionID != "" {
		if err := t.bindRevisionOwner(track.ContractID, track.CandidateRevisionID, "release track "+key+" candidate"); err != nil {
			return err
		}
	}
	t.state.ReleaseTracks[key] = domain.CloneReleaseTrack(track)
	t.state.ReleaseTrackAuthorities[key] = newReleaseTrackAuthority(track)
	t.validatedReleaseTracks[key] = domain.CloneReleaseTrack(track)
	return nil
}

func (t *operationalTransaction) SavePublication(ctx context.Context, publication domain.Publication) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateID(publication.ProjectID); err != nil {
		return err
	}
	if err := validateID(publication.RevisionID); err != nil {
		return err
	}
	if err := validatePublicPath(publication.Path); err != nil {
		return err
	}
	key := publicationKey(publication.ProjectID, publication.RevisionID)
	if err := t.bindRevisionOwner(publication.ProjectID, publication.RevisionID, "publication "+key); err != nil {
		return err
	}
	if publication.Public {
		for existingKey, existing := range t.state.Publications {
			if existingKey == key || !existing.Public || existing.Path != publication.Path {
				continue
			}
			existing.Public = false
			t.state.Publications[existingKey] = existing
		}
	}
	t.state.Publications[key] = publication
	return nil
}

func (t *operationalTransaction) AppendAuditEvent(ctx context.Context, event domain.AuditEvent) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateID(event.ID); err != nil {
		return err
	}
	if event.RevisionID != "" {
		if err := t.bindRevisionOwner(event.ContractID, event.RevisionID, "audit event "+event.ID); err != nil {
			return err
		}
	}
	t.state.AuditEvents[event.ID] = event
	return nil
}

func (t *operationalTransaction) Enqueue(ctx context.Context, message domain.OutboxMessage) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateID(message.ID); err != nil {
		return err
	}
	if message.RevisionID != "" {
		if err := t.bindRevisionOwner(message.ContractID, message.RevisionID, "outbox message "+message.ID); err != nil {
			return err
		}
	}
	t.state.Outbox[message.ID] = message
	return nil
}

func (t *operationalTransaction) bindRevisionOwner(contractID, revisionID, owner string) error {
	changed, err := bindRevisionOwner(&t.state, contractID, revisionID, owner)
	if err != nil {
		return err
	}
	if changed {
		t.mutatedRevisions[revisionID] = struct{}{}
	}
	return nil
}

func newOperationalState() operationalState {
	state := operationalState{Version: operationalStateVersion}
	state.initializeMaps()
	return state
}

func (s *operationalState) initializeMaps() {
	if s.Revisions == nil {
		s.Revisions = make(map[string]domain.ContractRevision)
	}
	if s.Reviews == nil {
		s.Reviews = make(map[string]domain.ContractReview)
	}
	if s.SyncRecords == nil {
		s.SyncRecords = make(map[string]domain.SyncRecord)
	}
	if s.ReleaseTracks == nil {
		s.ReleaseTracks = make(map[string]domain.ReleaseTrack)
	}
	if s.ReleaseTrackAuthorities == nil {
		s.ReleaseTrackAuthorities = make(map[string]releaseTrackAuthority)
	}
	if s.Publications == nil {
		s.Publications = make(map[string]domain.Publication)
	}
	if s.AuditEvents == nil {
		s.AuditEvents = make(map[string]domain.AuditEvent)
	}
	if s.Outbox == nil {
		s.Outbox = make(map[string]domain.OutboxMessage)
	}
	if s.migratedRevisions == nil {
		s.migratedRevisions = make(map[string]struct{})
	}
}

func cloneIDSet(values map[string]struct{}) map[string]struct{} {
	cloned := make(map[string]struct{}, len(values))
	for value := range values {
		cloned[value] = struct{}{}
	}
	return cloned
}

func cloneOperationalState(state operationalState) operationalState {
	cloned := state
	cloned.ReleaseTracks = make(map[string]domain.ReleaseTrack, len(state.ReleaseTracks))
	for key, track := range state.ReleaseTracks {
		cloned.ReleaseTracks[key] = domain.CloneReleaseTrack(track)
	}
	cloned.ReleaseTrackAuthorities = make(map[string]releaseTrackAuthority, len(state.ReleaseTrackAuthorities))
	for key, authority := range state.ReleaseTrackAuthorities {
		cloned.ReleaseTrackAuthorities[key] = authority
	}
	cloned.migratedRevisions = cloneIDSet(state.migratedRevisions)
	return cloned
}

func newReleaseTrackAuthority(track domain.ReleaseTrack) releaseTrackAuthority {
	authority := releaseTrackAuthority{
		Version:         releaseTrackAuthorityVersion,
		Generation:      track.Generation,
		DecisionPresent: track.LastDecision != nil,
	}
	if track.LastDecision != nil {
		authority.Decision = *track.LastDecision
	}
	return authority
}

func validateReleaseTrackAuthority(key string, track domain.ReleaseTrack, authority releaseTrackAuthority) error {
	if authority.Version != releaseTrackAuthorityVersion {
		return fmt.Errorf("release track %q authority version %d is unsupported", key, authority.Version)
	}
	if authority.Generation != track.Generation {
		return fmt.Errorf("release track %q authority generation %d does not match track generation %d", key, authority.Generation, track.Generation)
	}
	if track.LastDecision == nil {
		if authority.DecisionPresent || authority.Decision != (domain.ReleaseDecision{}) {
			return fmt.Errorf("release track %q baseline authority contains decision evidence", key)
		}
		return nil
	}
	if !authority.DecisionPresent || authority.Decision != *track.LastDecision {
		return fmt.Errorf("release track %q decision authority does not match its latest decision", key)
	}
	return nil
}

func validateReleaseTrackAuthorities(state operationalState) error {
	if len(state.ReleaseTrackAuthorities) != len(state.ReleaseTracks) {
		return fmt.Errorf("release track authority count does not match release track count")
	}
	for key, track := range state.ReleaseTracks {
		if err := domain.ValidateReleaseTrack(track); err != nil {
			return fmt.Errorf("release track %q is invalid: %w", key, err)
		}
		authority, ok := state.ReleaseTrackAuthorities[key]
		if !ok {
			return fmt.Errorf("release track %q has no decision authority marker", key)
		}
		if err := validateReleaseTrackAuthority(key, track, authority); err != nil {
			return err
		}
	}
	for key := range state.ReleaseTrackAuthorities {
		if _, ok := state.ReleaseTracks[key]; !ok {
			return fmt.Errorf("release track authority %q has no track", key)
		}
	}
	return nil
}

func migrateOperationalStateV1(state *operationalState) error {
	if state.Version != legacyOperationalStateVersion {
		return fmt.Errorf("cannot migrate operational state version %d", state.Version)
	}
	state.ReleaseTrackAuthorities = make(map[string]releaseTrackAuthority, len(state.ReleaseTracks))
	for key, track := range state.ReleaseTracks {
		if track.LastDecision == nil {
			track.CandidateRevisionID = ""
			state.ReleaseTracks[key] = track
		}
		if err := domain.ValidateReleaseTrack(track); err != nil {
			return fmt.Errorf("migrate v1 release track %q: %w", key, err)
		}
		state.ReleaseTrackAuthorities[key] = newReleaseTrackAuthority(track)
	}
	if err := bindLegacyOperationalRevisionOwners(state); err != nil {
		return fmt.Errorf("migrate v1 revision ownership: %w", err)
	}
	state.Version = operationalStateVersion
	return nil
}

func bindLegacyOperationalRevisionOwners(state *operationalState) error {
	for key, track := range state.ReleaseTracks {
		for role, revisionID := range map[string]string{
			"current": track.CurrentRevisionID, "candidate": track.CandidateRevisionID,
		} {
			if revisionID == "" {
				continue
			}
			if _, err := bindRevisionOwner(state, track.ContractID, revisionID, "release track "+key+" "+role); err != nil {
				return err
			}
		}
	}
	for key, publication := range state.Publications {
		if _, err := bindRevisionOwner(state, publication.ProjectID, publication.RevisionID, "publication "+key); err != nil {
			return err
		}
	}
	for key, review := range state.Reviews {
		if _, err := bindRevisionOwner(state, review.ContractID, review.BaselineRevisionID, "review "+key+" baseline"); err != nil {
			return err
		}
		if _, err := bindRevisionOwner(state, review.ContractID, review.CandidateRevisionID, "review "+key+" candidate"); err != nil {
			return err
		}
	}
	for key, record := range state.SyncRecords {
		if record.Result == domain.SyncResultSuccess && record.RevisionID != "" {
			if _, err := bindRevisionOwner(state, record.ProjectID, record.RevisionID, "sync record "+key); err != nil {
				return err
			}
		}
	}
	for key, event := range state.AuditEvents {
		if event.RevisionID != "" {
			if _, err := bindRevisionOwner(state, event.ContractID, event.RevisionID, "audit event "+key); err != nil {
				return err
			}
		}
	}
	for key, message := range state.Outbox {
		if message.RevisionID != "" {
			if _, err := bindRevisionOwner(state, message.ContractID, message.RevisionID, "outbox message "+key); err != nil {
				return err
			}
		}
	}
	return nil
}

func bindRevisionOwner(state *operationalState, contractID, revisionID, owner string) (bool, error) {
	if strings.TrimSpace(contractID) == "" {
		return false, fmt.Errorf("%s has no owning contract", owner)
	}
	revision, ok := state.Revisions[revisionID]
	if !ok {
		return false, fmt.Errorf("%s references uncommitted revision %q", owner, revisionID)
	}
	if revision.ContractID == contractID {
		return false, nil
	}
	if revision.ContractID != "" {
		return false, fmt.Errorf("%s references revision %q owned by contract %q, not %q", owner, revisionID, revision.ContractID, contractID)
	}
	revision.ContractID = contractID
	state.Revisions[revisionID] = revision
	return true, nil
}

func validateOperationalReferences(state operationalState) error {
	for key, revision := range state.Revisions {
		if key != revision.ID {
			return fmt.Errorf("revision key %q does not match revision id %q", key, revision.ID)
		}
	}
	for key, track := range state.ReleaseTracks {
		if key != releaseTrackKey(track.ContractID, track.ID) {
			return fmt.Errorf("release track key %q does not match its identity", key)
		}
		for role, revisionID := range map[string]string{
			"current": track.CurrentRevisionID, "candidate": track.CandidateRevisionID,
		} {
			if revisionID == "" {
				continue
			}
			if err := requireOwnedRevision(state, track.ContractID, revisionID, "release track "+key+" "+role); err != nil {
				return err
			}
		}
	}
	for key, publication := range state.Publications {
		if key != publicationKey(publication.ProjectID, publication.RevisionID) {
			return fmt.Errorf("publication key %q does not match its identity", key)
		}
		if err := requireOwnedRevision(state, publication.ProjectID, publication.RevisionID, "publication "+key); err != nil {
			return err
		}
	}
	for key, review := range state.Reviews {
		if key != review.ID {
			return fmt.Errorf("review key %q does not match review id %q", key, review.ID)
		}
		if err := requireOwnedRevision(state, review.ContractID, review.BaselineRevisionID, "review "+key+" baseline"); err != nil {
			return err
		}
		if err := requireOwnedRevision(state, review.ContractID, review.CandidateRevisionID, "review "+key+" candidate"); err != nil {
			return err
		}
	}
	for key, record := range state.SyncRecords {
		if key != record.ID {
			return fmt.Errorf("sync record key %q does not match record id %q", key, record.ID)
		}
		if record.Result == domain.SyncResultSuccess && record.RevisionID != "" {
			if err := requireOwnedRevision(state, record.ProjectID, record.RevisionID, "sync record "+key); err != nil {
				return err
			}
		}
	}
	for key, event := range state.AuditEvents {
		if key != event.ID {
			return fmt.Errorf("audit event key %q does not match event id %q", key, event.ID)
		}
		if event.RevisionID != "" {
			if err := requireOwnedRevision(state, event.ContractID, event.RevisionID, "audit event "+key); err != nil {
				return err
			}
		}
		if event.TrackID != "" {
			if _, ok := state.ReleaseTracks[releaseTrackKey(event.ContractID, event.TrackID)]; !ok {
				return fmt.Errorf("audit event %q references an uncommitted release track", key)
			}
		}
	}
	for key, message := range state.Outbox {
		if key != message.ID {
			return fmt.Errorf("outbox message key %q does not match message id %q", key, message.ID)
		}
		if message.RevisionID != "" {
			if err := requireOwnedRevision(state, message.ContractID, message.RevisionID, "outbox message "+key); err != nil {
				return err
			}
		}
		if message.TrackID != "" {
			if _, ok := state.ReleaseTracks[releaseTrackKey(message.ContractID, message.TrackID)]; !ok {
				return fmt.Errorf("outbox message %q references an uncommitted release track", key)
			}
		}
	}
	return nil
}

func requireOwnedRevision(state operationalState, contractID, revisionID, owner string) error {
	if strings.TrimSpace(contractID) == "" {
		return fmt.Errorf("%s has no owning contract", owner)
	}
	revision, ok := state.Revisions[revisionID]
	if !ok {
		return fmt.Errorf("%s references uncommitted revision %q", owner, revisionID)
	}
	if revision.ContractID != contractID {
		return fmt.Errorf("%s references revision %q owned by contract %q, not %q", owner, revisionID, revision.ContractID, contractID)
	}
	return nil
}

func cloneReleaseTracks(tracks map[string]domain.ReleaseTrack) map[string]domain.ReleaseTrack {
	cloned := make(map[string]domain.ReleaseTrack, len(tracks))
	for key, track := range tracks {
		cloned[key] = domain.CloneReleaseTrack(track)
	}
	return cloned
}

func validateCommittedReleaseTracks(validated, committed map[string]domain.ReleaseTrack) error {
	if !reflect.DeepEqual(validated, committed) {
		return fmt.Errorf("committed release tracks differ from validated transitions")
	}
	return nil
}

func validateReleaseTrackInitialization(track domain.ReleaseTrack) error {
	if err := domain.ValidateReleaseTrack(track); err != nil {
		return err
	}
	if track.LastDecision == nil {
		if track.Generation != 0 {
			return fmt.Errorf("release track baseline initialization requires generation zero")
		}
		return nil
	}
	if track.Generation != 1 {
		return fmt.Errorf("first release decision requires generation one")
	}
	baseline := domain.CloneReleaseTrack(track)
	baseline.Generation = 0
	baseline.CandidateRevisionID = ""
	baseline.LastDecision = nil
	if track.Mode == domain.ReleaseModeFollowing && track.LastDecision.Accepted {
		baseline.CurrentRevisionID = ""
	}
	expected, changed, err := domain.ConsiderReleaseDecision(baseline, *track.LastDecision)
	if err != nil {
		return fmt.Errorf("derive first release decision: %w", err)
	}
	if !changed || !reflect.DeepEqual(expected, track) {
		return fmt.Errorf("release track does not match a first authoritative decision")
	}
	return nil
}

func releaseTrackKey(contractID, trackID string) string {
	return contractID + "\x00" + trackID
}

func publicationKey(projectID, revisionID string) string {
	return projectID + "\x00" + revisionID
}

func writeFileAtomically(filePath string, data []byte, mode fs.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(filePath), ".write-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	removeTemporary := true
	defer func() {
		_ = temporary.Close()
		if removeTemporary {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(mode); err != nil {
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, filePath); err != nil {
		return err
	}
	removeTemporary = false
	directory, err := os.Open(filepath.Dir(filePath))
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

func validateID(id string) error {
	if id == "" || id == "." || id == ".." || filepath.IsAbs(id) {
		return errUnsafeStorePath
	}
	if strings.ContainsAny(id, `/\`) || filepath.Clean(id) != id {
		return errUnsafeStorePath
	}
	return nil
}

func validatePublicPath(publicPath string) error {
	if publicPath == "" || !strings.HasPrefix(publicPath, "/") || strings.Contains(publicPath, `\`) {
		return errUnsafeStorePath
	}
	if path.Clean(publicPath) != publicPath {
		return errUnsafeStorePath
	}
	return nil
}

var _ port.UnitOfWork = (*FileStore)(nil)
var _ port.BlobStore = (*FileStore)(nil)
var _ port.RevisionReader = (*FileStore)(nil)
var _ port.OperationalStore = (*operationalTransaction)(nil)
