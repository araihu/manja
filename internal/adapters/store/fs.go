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
	"strings"
	"sync"

	"github.com/araihu/manja/application/port"
	"github.com/araihu/manja/domain"
)

var errUnsafeStorePath = errors.New("unsafe store path")

const operationalStateVersion = 1

type FileStore struct {
	root string
	mu   sync.Mutex
}

type operationalState struct {
	Version       int                                `json:"version"`
	Revisions     map[string]domain.ContractRevision `json:"revisions"`
	Reviews       map[string]domain.ContractReview   `json:"reviews"`
	SyncRecords   map[string]domain.SyncRecord       `json:"syncRecords"`
	ReleaseTracks map[string]domain.ReleaseTrack     `json:"releaseTracks"`
	Publications  map[string]domain.Publication      `json:"publications"`
	AuditEvents   map[string]domain.AuditEvent       `json:"auditEvents"`
	Outbox        map[string]domain.OutboxMessage    `json:"outbox"`
}

func NewFileStore(root string) *FileStore {
	store := &FileStore{root: root}
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
		state:            state,
		mutatedRevisions: make(map[string]struct{}),
	}
	if err := callback(ctx, transaction); err != nil {
		return err
	}
	if err := s.validateOperationalState(ctx, transaction.state, transaction.mutatedRevisions); err != nil {
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
	return track, nil
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
		if state.Version != operationalStateVersion {
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
	return state, nil
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
	directory, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

func (s *FileStore) validateOperationalState(ctx context.Context, state operationalState, mutatedRevisions map[string]struct{}) error {
	for id := range mutatedRevisions {
		revision := state.Revisions[id]
		if revision.SpecBlobKey == "" {
			continue
		}
		key := port.BlobKey(revision.SpecBlobKey)
		if !key.Valid() {
			return fmt.Errorf("revision %q has invalid blob key %q", id, key)
		}
		path, err := s.blobPath(key)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("revision %q references missing blob %q: %w", id, key, err)
		}
		if port.ContentAddressedBlobKey(data) != key {
			return fmt.Errorf("revision %q references corrupt blob %q", id, key)
		}
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	for key, track := range state.ReleaseTracks {
		for _, revisionID := range []string{track.CurrentRevisionID, track.CandidateRevisionID} {
			if revisionID == "" {
				continue
			}
			if _, ok := state.Revisions[revisionID]; !ok {
				return fmt.Errorf("release track %q references uncommitted revision %q", key, revisionID)
			}
		}
	}
	for key, publication := range state.Publications {
		if _, ok := state.Revisions[publication.RevisionID]; !ok {
			return fmt.Errorf("publication %q references uncommitted revision %q", key, publication.RevisionID)
		}
	}
	for id, review := range state.Reviews {
		if _, ok := state.Revisions[review.CandidateRevisionID]; !ok {
			return fmt.Errorf("review %q references uncommitted revision %q", id, review.CandidateRevisionID)
		}
	}
	for id, record := range state.SyncRecords {
		if record.Result == domain.SyncResultSuccess && record.RevisionID != "" {
			if _, ok := state.Revisions[record.RevisionID]; !ok {
				return fmt.Errorf("sync record %q references uncommitted revision %q", id, record.RevisionID)
			}
		}
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
	state            operationalState
	mutatedRevisions map[string]struct{}
}

func (t *operationalTransaction) SaveRevision(ctx context.Context, revision domain.ContractRevision) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateID(revision.ID); err != nil {
		return err
	}
	t.state.Revisions[revision.ID] = revision
	t.mutatedRevisions[revision.ID] = struct{}{}
	return nil
}

func (t *operationalTransaction) SaveReview(ctx context.Context, review domain.ContractReview) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateID(review.ID); err != nil {
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
	return track, nil
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
	key := releaseTrackKey(track.ContractID, track.ID)
	current, exists := t.state.ReleaseTracks[key]
	if exists && current.Generation != expectedGeneration {
		return port.ErrGenerationConflict
	}
	if !exists && expectedGeneration != 0 {
		return port.ErrGenerationConflict
	}
	t.state.ReleaseTracks[key] = track
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
	t.state.Publications[publicationKey(publication.ProjectID, publication.RevisionID)] = publication
	return nil
}

func (t *operationalTransaction) AppendAuditEvent(ctx context.Context, event domain.AuditEvent) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateID(event.ID); err != nil {
		return err
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
	t.state.Outbox[message.ID] = message
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
	if s.Publications == nil {
		s.Publications = make(map[string]domain.Publication)
	}
	if s.AuditEvents == nil {
		s.AuditEvents = make(map[string]domain.AuditEvent)
	}
	if s.Outbox == nil {
		s.Outbox = make(map[string]domain.OutboxMessage)
	}
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
var _ port.OperationalStore = (*operationalTransaction)(nil)
