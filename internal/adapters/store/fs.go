package store

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	"time"
	"unicode"

	"github.com/araihu/manja/application/port"
	"github.com/araihu/manja/domain"
)

var errUnsafeStorePath = errors.New("unsafe store path")

const (
	legacyOperationalStateVersion   = 1
	decisionOperationalStateVersion = 2
	authenticatedStateVersion       = 3
	operationalStateVersion         = 4
	releaseTrackAuthorityVersion    = 1
	atomicWriteStagingPattern       = ".write-*.tmp"
)

type operationalSchemaMarker struct {
	Version int `json:"version"`
}

type releaseTrackAuthority struct {
	Version         int                    `json:"version"`
	Generation      uint64                 `json:"generation"`
	DecisionPresent bool                   `json:"decisionPresent"`
	Decision        domain.ReleaseDecision `json:"decision"`
}

type FileStore struct {
	root               string
	admission          chan struct{}
	confirmReplacement func(string) error
}

type operationalState struct {
	Version                 int                                    `json:"version"`
	Revisions               map[string]domain.ContractRevision     `json:"revisions"`
	Reviews                 map[string]domain.ContractReview       `json:"reviews"`
	SyncRecords             map[string]domain.SyncRecord           `json:"syncRecords"`
	ReleaseTracks           map[string]domain.ReleaseTrack         `json:"releaseTracks"`
	ReleaseTrackAuthorities map[string]releaseTrackAuthority       `json:"releaseTrackAuthorities"`
	ReleaseAuthorizations   map[string]domain.ReleaseAuthorization `json:"releaseAuthorizations"`
	Publications            map[string]domain.Publication          `json:"publications"`
	AuditEvents             map[string]domain.AuditEvent           `json:"auditEvents"`
	Outbox                  map[string]domain.OutboxMessage        `json:"outbox"`
	migratedRevisions       map[string]struct{}
}

func NewFileStore(root string) *FileStore {
	store := &FileStore{
		root:               root,
		admission:          make(chan struct{}, 1),
		confirmReplacement: confirmAtomicReplacement,
	}
	store.admission <- struct{}{}
	return store
}

// acquireAdmission serializes all operations performed through one FileStore
// handle without making context cancellation wait behind another callback. The
// admission is intentionally acquired before the inter-process lock so lock
// ordering remains stable and callbacks remain non-reentrant.
func (s *FileStore) acquireAdmission(ctx context.Context) (func(), error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-s.admission:
		return func() { s.admission <- struct{}{} }, nil
	}
}

func (s *FileStore) Within(ctx context.Context, callback func(context.Context, port.OperationalStore) error) error {
	if callback == nil {
		return fmt.Errorf("operational callback is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	releaseAdmission, err := s.acquireAdmission(ctx)
	if err != nil {
		return err
	}
	defer releaseAdmission()
	releaseLock, err := s.acquireOperationalLock(ctx)
	if err != nil {
		return err
	}
	defer releaseLock()
	s.discardIncompleteOperationalStaging()

	state, err := s.loadOperationalStateLocked(ctx)
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
	if err := s.validateOperationalState(ctx, state, transaction.state, transaction.mutatedRevisions); err != nil {
		return err
	}
	if err := validateCommittedReleaseTracks(transaction.validatedReleaseTracks, transaction.state.ReleaseTracks); err != nil {
		return err
	}
	return s.publishCurrentOperationalState(ctx, transaction.state)
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
	if err := s.readJSON(ctx, "projects", id+".json", &project); err != nil {
		return project, err
	}
	if project.ID != id {
		return domain.Project{}, fmt.Errorf("project lookup %q returned persisted id %q", id, project.ID)
	}
	return project, nil
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
	releaseAdmission, err := s.acquireAdmission(ctx)
	if err != nil {
		return revision, err
	}
	defer releaseAdmission()
	state, err := s.loadOperationalState(ctx)
	if err != nil {
		return revision, err
	}
	matches := revisionsByID(state, id)
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) > 1 {
		return domain.ContractRevision{}, fmt.Errorf("revision %q is ambiguous across contracts", id)
	}
	if err := s.readJSON(ctx, "revisions", id+".json", &revision); err != nil {
		return revision, err
	}
	if revision.ID != id {
		return domain.ContractRevision{}, fmt.Errorf("revision lookup %q returned persisted id %q", id, revision.ID)
	}
	return revision, nil
}

func (s *FileStore) ContractRevision(ctx context.Context, contractID, revisionID string) (domain.ContractRevision, error) {
	if err := validateID(contractID); err != nil {
		return domain.ContractRevision{}, err
	}
	if err := validateID(revisionID); err != nil {
		return domain.ContractRevision{}, err
	}
	releaseAdmission, err := s.acquireAdmission(ctx)
	if err != nil {
		return domain.ContractRevision{}, err
	}
	defer releaseAdmission()
	state, err := s.loadOperationalState(ctx)
	if err != nil {
		return domain.ContractRevision{}, err
	}
	revision, ok := revisionForContract(state, contractID, revisionID)
	if !ok {
		return domain.ContractRevision{}, fs.ErrNotExist
	}
	if revision.ContractID != contractID {
		return domain.ContractRevision{}, fmt.Errorf("revision %q belongs to contract %q, not %q", revisionID, revision.ContractID, contractID)
	}
	if revision.ID != revisionID {
		return domain.ContractRevision{}, fmt.Errorf("revision lookup %q returned persisted id %q", revisionID, revision.ID)
	}
	if err := s.validateRevisionEvidence(ctx, revision); err != nil {
		return domain.ContractRevision{}, fmt.Errorf("revision %q has invalid persisted evidence: %w", revisionID, err)
	}
	return revision, nil
}

func (s *FileStore) SaveReleaseAuthorization(ctx context.Context, authorization domain.ReleaseAuthorization) error {
	return s.Within(ctx, func(ctx context.Context, operational port.OperationalStore) error {
		transaction, ok := operational.(*operationalTransaction)
		if !ok {
			return fmt.Errorf("file store release authorization transaction is unavailable")
		}
		return transaction.saveReleaseAuthorization(ctx, authorization)
	})
}

func (s *FileStore) ReleaseEvidence(
	ctx context.Context,
	contractID, trackID, reviewID string,
) (domain.ReleaseEvidence, error) {
	for _, identity := range []string{contractID, trackID, reviewID} {
		if err := validateID(identity); err != nil {
			return domain.ReleaseEvidence{}, err
		}
	}
	releaseAdmission, err := s.acquireAdmission(ctx)
	if err != nil {
		return domain.ReleaseEvidence{}, err
	}
	defer releaseAdmission()
	state, err := s.loadOperationalState(ctx)
	if err != nil {
		return domain.ReleaseEvidence{}, err
	}
	authorization, ok := state.ReleaseAuthorizations[reviewID]
	if !ok {
		return domain.ReleaseEvidence{}, fs.ErrNotExist
	}
	if authorization.ContractID != contractID || authorization.TrackID != trackID {
		return domain.ReleaseEvidence{}, fmt.Errorf("release authorization %q belongs to another track", reviewID)
	}
	review, ok := state.Reviews[authorization.ReviewID]
	if !ok {
		return domain.ReleaseEvidence{}, fmt.Errorf("release authorization %q has no persisted review", reviewID)
	}
	syncRecord, ok := state.SyncRecords[authorization.SyncRecordID]
	if !ok {
		return domain.ReleaseEvidence{}, fmt.Errorf("release authorization %q has no persisted sync", reviewID)
	}
	if err := s.validateReleaseAuthorizationRevisionEvidence(ctx, state, authorization); err != nil {
		return domain.ReleaseEvidence{}, fmt.Errorf("release authorization %q has unavailable evidence: %w", reviewID, err)
	}
	return domain.ReleaseEvidence{
		Authorization: authorization,
		Review:        review,
		SyncRecord:    syncRecord,
	}, nil
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
	releaseAdmission, err := s.acquireAdmission(ctx)
	if err != nil {
		return publication, err
	}
	defer releaseAdmission()
	state, err := s.loadOperationalState(ctx)
	if err != nil {
		return publication, err
	}
	if publication, ok := state.Publications[publicationKey(projectID, revisionID)]; ok {
		if publication.ProjectID != projectID || publication.RevisionID != revisionID {
			return domain.Publication{}, fmt.Errorf("publication lookup identity does not match persisted record")
		}
		return publication, nil
	}
	if err := s.readJSON(ctx, "publications", projectID+"-"+revisionID+".json", &publication); err != nil {
		return publication, err
	}
	if publication.ProjectID != projectID || publication.RevisionID != revisionID {
		return domain.Publication{}, fmt.Errorf("publication lookup identity does not match persisted record")
	}
	return publication, nil
}

func (s *FileStore) PublicPublicationByPath(ctx context.Context, publicPath string) (domain.Publication, error) {
	var publication domain.Publication
	if err := validatePublicPath(publicPath); err != nil {
		return publication, err
	}
	if err := ctx.Err(); err != nil {
		return publication, err
	}
	releaseAdmission, err := s.acquireAdmission(ctx)
	if err != nil {
		return publication, err
	}
	defer releaseAdmission()
	state, err := s.loadOperationalState(ctx)
	if err != nil {
		return publication, err
	}
	for _, candidate := range state.Publications {
		if candidate.Public && candidate.Path == publicPath {
			if err := s.validatePublicationRevisionEvidence(ctx, state, candidate); err != nil {
				return domain.Publication{}, err
			}
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
			if err := s.validatePublicationRevisionEvidence(ctx, state, candidate); err != nil {
				return domain.Publication{}, err
			}
			return candidate, nil
		}
	}
	return publication, fs.ErrNotExist
}

func (s *FileStore) validatePublicationRevisionEvidence(
	ctx context.Context,
	state operationalState,
	publication domain.Publication,
) error {
	revision, ok := revisionForContract(state, publication.ProjectID, publication.RevisionID)
	if !ok {
		return fmt.Errorf("public publication references missing contract revision")
	}
	if err := s.validateRevisionEvidence(ctx, revision); err != nil {
		return fmt.Errorf("public publication revision evidence is unavailable: %w", err)
	}
	return nil
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
	releaseAdmission, err := s.acquireAdmission(ctx)
	if err != nil {
		return record, err
	}
	defer releaseAdmission()
	state, err := s.loadOperationalState(ctx)
	if err != nil {
		return record, err
	}
	if record, ok := state.SyncRecords[id]; ok {
		if record.ID != id {
			return domain.SyncRecord{}, fmt.Errorf("sync lookup %q returned persisted id %q", id, record.ID)
		}
		return record, nil
	}
	if err := s.readJSON(ctx, "sync-history", id+".json", &record); err != nil {
		return record, err
	}
	if record.ID != id {
		return domain.SyncRecord{}, fmt.Errorf("sync lookup %q returned persisted id %q", id, record.ID)
	}
	return record, nil
}

func (s *FileStore) ReleaseTrack(ctx context.Context, contractID, trackID string) (domain.ReleaseTrack, error) {
	var track domain.ReleaseTrack
	if err := validateID(contractID); err != nil {
		return track, err
	}
	if err := validateID(trackID); err != nil {
		return track, err
	}
	releaseAdmission, err := s.acquireAdmission(ctx)
	if err != nil {
		return track, err
	}
	defer releaseAdmission()
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
	if err := durableAtomicWrite(path, data, 0o600); err != nil {
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
	releaseLock, err := s.acquireOperationalLock(ctx)
	if err != nil {
		return operationalState{}, err
	}
	defer releaseLock()
	s.discardIncompleteOperationalStaging()
	return s.loadOperationalStateLocked(ctx)
}

func (s *FileStore) loadOperationalStateLocked(ctx context.Context) (operationalState, error) {
	state := newOperationalState()
	if err := ctx.Err(); err != nil {
		return state, err
	}
	path := filepath.Join(s.root, "operational", "state.json")
	marker, markerPresent, err := s.loadOperationalSchemaMarker(ctx)
	if err != nil {
		return state, err
	}
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
		case legacyOperationalStateVersion, decisionOperationalStateVersion:
			previousVersion := state.Version
			if len(state.ReleaseAuthorizations) != 0 {
				return operationalState{}, fmt.Errorf(
					"operational state version %d contains authenticated release authority",
					state.Version,
				)
			}
			if markerPresent && marker.Version >= authenticatedStateVersion {
				return operationalState{}, fmt.Errorf(
					"operational state version %d is older than durable schema marker %d",
					state.Version,
					marker.Version,
				)
			}
			if err := migrateOperationalStateToCurrent(&state); err != nil {
				return operationalState{}, err
			}
			if err := validateReleaseTrackAuthorities(state); err != nil {
				return operationalState{}, err
			}
			if err := validateOperationalReferences(state); err != nil {
				return operationalState{}, err
			}
			if err := s.validateReferencedRevisionEvidence(ctx, state, collectLegacyRevisionReferences(state)); err != nil {
				return operationalState{}, err
			}
			if err := s.publishCurrentOperationalState(ctx, state); err != nil {
				return operationalState{}, fmt.Errorf(
					"persist operational state v%d to v%d migration: %w",
					previousVersion,
					operationalStateVersion,
					err,
				)
			}
		case authenticatedStateVersion:
			if markerPresent && marker.Version > authenticatedStateVersion {
				return operationalState{}, fmt.Errorf(
					"operational state version %d is older than durable schema marker %d",
					state.Version,
					marker.Version,
				)
			}
			if err := migrateOperationalStateToCurrent(&state); err != nil {
				return operationalState{}, err
			}
			if err := validateReleaseTrackAuthorities(state); err != nil {
				return operationalState{}, err
			}
			if err := validateOperationalReferences(state); err != nil {
				return operationalState{}, err
			}
			if err := s.validateReferencedRevisionEvidence(ctx, state, collectLegacyRevisionReferences(state)); err != nil {
				return operationalState{}, err
			}
			if err := s.publishCurrentOperationalState(ctx, state); err != nil {
				return operationalState{}, fmt.Errorf(
					"persist authenticated operational state v%d to v%d migration: %w",
					authenticatedStateVersion,
					operationalStateVersion,
					err,
				)
			}
		case operationalStateVersion:
			if markerPresent && marker.Version > operationalStateVersion {
				return operationalState{}, fmt.Errorf("unsupported durable operational schema marker %d", marker.Version)
			}
			if err := validateReleaseTrackAuthorities(state); err != nil {
				return operationalState{}, err
			}
			if err := validateOperationalReferences(state); err != nil {
				return operationalState{}, err
			}
			if !markerPresent || marker.Version < operationalStateVersion {
				if err := s.persistOperationalSchemaMarker(ctx, operationalStateVersion); err != nil {
					return operationalState{}, fmt.Errorf("recover operational schema marker: %w", err)
				}
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
		return revisionStorageKey(revision)
	}); err != nil {
		return operationalState{}, err
	}
	for key := range state.Revisions {
		state.migratedRevisions[key] = struct{}{}
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
	revisionKeys := make([]string, 0, len(state.Revisions))
	for key := range state.Revisions {
		revisionKeys = append(revisionKeys, key)
	}
	sort.Strings(revisionKeys)
	awaitingEvidenceEnrichment := false
	for _, key := range revisionKeys {
		revision := state.Revisions[key]
		if err := s.validateRevisionEvidence(ctx, revision); err != nil {
			if legacyRevisionAwaitsEvidenceEnrichment(revision) {
				awaitingEvidenceEnrichment = true
				continue
			}
			return operationalState{}, fmt.Errorf("validate legacy revision %q migration: %w", revision.ID, err)
		}
	}
	if awaitingEvidenceEnrichment {
		// A flat pre-snapshot record may be completed by SaveRevision in this
		// transaction. It cannot be published as the current schema until that enrichment has
		// passed the normal final commit validation.
		return state, nil
	}
	if err := s.publishCurrentOperationalState(ctx, state); err != nil {
		return operationalState{}, fmt.Errorf("persist legacy operational state migration: %w", err)
	}
	return state, nil
}

func (s *FileStore) publishCurrentOperationalState(ctx context.Context, state operationalState) error {
	if state.Version != operationalStateVersion {
		return fmt.Errorf("cannot publish non-current operational state version %d", state.Version)
	}
	if err := s.publishOperationalState(ctx, state); err != nil {
		return err
	}
	if err := s.persistOperationalSchemaMarker(ctx, operationalStateVersion); err != nil {
		return fmt.Errorf("%w: persist operational schema marker after state commit: %v", port.ErrCommitOutcomeUnknown, err)
	}
	return nil
}

func (s *FileStore) loadOperationalSchemaMarker(ctx context.Context) (operationalSchemaMarker, bool, error) {
	if err := ctx.Err(); err != nil {
		return operationalSchemaMarker{}, false, err
	}
	data, err := os.ReadFile(filepath.Join(s.root, "operational", "schema.json"))
	if errors.Is(err, fs.ErrNotExist) {
		return operationalSchemaMarker{}, false, nil
	}
	if err != nil {
		return operationalSchemaMarker{}, false, err
	}
	var marker operationalSchemaMarker
	if err := json.Unmarshal(data, &marker); err != nil {
		return operationalSchemaMarker{}, false, fmt.Errorf("decode operational schema marker: %w", err)
	}
	if marker.Version < authenticatedStateVersion {
		return operationalSchemaMarker{}, false, fmt.Errorf("invalid operational schema marker version %d", marker.Version)
	}
	return marker, true, nil
}

func (s *FileStore) persistOperationalSchemaMarker(ctx context.Context, version int) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	marker, present, err := s.loadOperationalSchemaMarker(ctx)
	if err != nil {
		return err
	}
	if present {
		if marker.Version > version {
			return fmt.Errorf("refusing to lower operational schema marker from %d to %d", marker.Version, version)
		}
		if marker.Version == version {
			return nil
		}
	}
	data, err := json.MarshalIndent(operationalSchemaMarker{Version: version}, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return durableAtomicWrite(filepath.Join(s.root, "operational", "schema.json"), data, 0o600)
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
	return durableAtomicWriteWithConfirmation(
		filepath.Join(dir, "state.json"),
		data,
		0o600,
		s.confirmReplacement,
	)
}

func (s *FileStore) validateOperationalState(
	ctx context.Context,
	previous, state operationalState,
	mutatedRevisions map[string]struct{},
) error {
	if state.Version != operationalStateVersion {
		return fmt.Errorf("operational state version %d is not current", state.Version)
	}
	if err := validateReleaseTrackAuthorities(state); err != nil {
		return err
	}
	if err := validateOperationalReferences(state); err != nil {
		return err
	}
	for key := range mutatedRevisions {
		revision, ok := state.Revisions[key]
		if !ok {
			return fmt.Errorf("mutated revision %q is missing", key)
		}
		if err := s.validateRevisionEvidence(ctx, revision); err != nil {
			return fmt.Errorf("revision %q has invalid persisted evidence: %w", revision.ID, err)
		}
	}
	if err := s.validateChangedReleaseEvidence(ctx, previous, state); err != nil {
		return err
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

func (s *FileStore) validateReleaseAuthorizationRevisionEvidence(
	ctx context.Context,
	state operationalState,
	authorization domain.ReleaseAuthorization,
) error {
	for _, revisionID := range []string{
		authorization.BaselineRevisionID,
		authorization.CandidateRevisionID,
	} {
		revision, ok := revisionForContract(state, authorization.ContractID, revisionID)
		if !ok {
			return fmt.Errorf("authorized revision %q is missing", revisionID)
		}
		if err := s.validateRevisionEvidence(ctx, revision); err != nil {
			return fmt.Errorf("authorized revision %q has invalid persisted evidence: %w", revisionID, err)
		}
	}
	return nil
}

func (s *FileStore) validateChangedReleaseEvidence(
	ctx context.Context,
	previous, state operationalState,
) error {
	for reviewID, authorization := range state.ReleaseAuthorizations {
		prior, existed := previous.ReleaseAuthorizations[reviewID]
		if existed && reflect.DeepEqual(prior, authorization) {
			continue
		}
		if err := validateAuthorizationBaseline(previous, authorization); err != nil {
			return fmt.Errorf("release authorization %q baseline binding: %w", reviewID, err)
		}
		if err := s.validateReleaseAuthorizationRevisionEvidence(ctx, state, authorization); err != nil {
			return fmt.Errorf("release authorization %q evidence: %w", reviewID, err)
		}
	}
	for key, track := range state.ReleaseTracks {
		prior, existed := previous.ReleaseTracks[key]
		if existed && reflect.DeepEqual(prior, track) {
			continue
		}
		if track.LastDecision == nil {
			continue
		}
		authorization, ok := state.ReleaseAuthorizations[track.LastDecision.ReviewID]
		if !ok {
			return fmt.Errorf("changed release track %q has no persisted authorization", key)
		}
		if err := validateAuthorizationBaseline(previous, authorization); err != nil {
			return fmt.Errorf("changed release track %q baseline binding: %w", key, err)
		}
		if err := s.validateReleaseAuthorizationRevisionEvidence(ctx, state, authorization); err != nil {
			return fmt.Errorf("changed release track %q evidence: %w", key, err)
		}
	}
	return nil
}

func validateAuthorizationBaseline(previous operationalState, authorization domain.ReleaseAuthorization) error {
	track, ok := previous.ReleaseTracks[releaseTrackKey(authorization.ContractID, authorization.TrackID)]
	if !ok {
		return fmt.Errorf("authorized release track has no persisted pre-transition state")
	}
	if track.CurrentRevisionID != authorization.BaselineRevisionID {
		return fmt.Errorf(
			"authorized baseline %q does not match pre-transition current revision %q",
			authorization.BaselineRevisionID,
			track.CurrentRevisionID,
		)
	}
	return nil
}

type legacyRevisionReference struct {
	contractID string
	revisionID string
	owner      string
}

func collectLegacyRevisionReferences(state operationalState) []legacyRevisionReference {
	var references []legacyRevisionReference
	appendReference := func(contractID, revisionID, owner string) {
		if revisionID == "" {
			return
		}
		references = append(references, legacyRevisionReference{
			contractID: contractID,
			revisionID: revisionID,
			owner:      owner,
		})
	}
	for key, track := range state.ReleaseTracks {
		appendReference(track.ContractID, track.CurrentRevisionID, "release track "+key+" current")
		appendReference(track.ContractID, track.CandidateRevisionID, "release track "+key+" candidate")
	}
	for key, publication := range state.Publications {
		appendReference(publication.ProjectID, publication.RevisionID, "publication "+key)
	}
	for key, record := range state.SyncRecords {
		if record.Result == domain.SyncResultSuccess {
			appendReference(record.ProjectID, record.RevisionID, "sync record "+key)
		}
	}
	for key, review := range state.Reviews {
		appendReference(review.ContractID, review.BaselineRevisionID, "review "+key+" baseline")
		appendReference(review.ContractID, review.CandidateRevisionID, "review "+key+" candidate")
	}
	for key, event := range state.AuditEvents {
		appendReference(event.ContractID, event.RevisionID, "audit event "+key)
	}
	for key, message := range state.Outbox {
		appendReference(message.ContractID, message.RevisionID, "outbox message "+key)
	}
	sort.Slice(references, func(i, j int) bool {
		left := references[i].contractID + "\x00" + references[i].revisionID + "\x00" + references[i].owner
		right := references[j].contractID + "\x00" + references[j].revisionID + "\x00" + references[j].owner
		return left < right
	})
	return references
}

func (s *FileStore) validateReferencedRevisionEvidence(
	ctx context.Context,
	state operationalState,
	references []legacyRevisionReference,
) error {
	for _, reference := range references {
		revision, ok := revisionForContract(state, reference.contractID, reference.revisionID)
		if !ok {
			return fmt.Errorf("%s references missing revision %q", reference.owner, reference.revisionID)
		}
		if err := s.validateRevisionEvidence(ctx, revision); err != nil {
			return fmt.Errorf("%s has invalid revision evidence: %w", reference.owner, err)
		}
	}
	return nil
}

func (s *FileStore) discardIncompleteOperationalStaging() {
	matches, err := filepath.Glob(filepath.Join(s.root, "operational", atomicWriteStagingPattern))
	if err == nil {
		for _, match := range matches {
			_ = os.Remove(match)
		}
	}
}

func (s *FileStore) acquireOperationalLock(ctx context.Context) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	directory := filepath.Join(s.root, "operational")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return nil, err
	}
	lockFile, err := os.OpenFile(filepath.Join(directory, ".lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	for {
		acquired, lockErr := tryOperationalFileLock(lockFile)
		if lockErr != nil {
			_ = lockFile.Close()
			return nil, lockErr
		}
		if acquired {
			return func() {
				_ = unlockOperationalFile(lockFile)
				_ = lockFile.Close()
			}, nil
		}
		timer := time.NewTimer(10 * time.Millisecond)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			_ = lockFile.Close()
			return nil, ctx.Err()
		case <-timer.C:
		}
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
	return durableAtomicWrite(path, data, 0o600)
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
	for _, identity := range []struct {
		name  string
		value string
	}{
		{"revision contract id", revision.ContractID},
		{"revision source id", revision.SourceID},
		{"revision ref", revision.Ref},
	} {
		if err := validateCanonicalIdentity(identity.name, identity.value, true); err != nil {
			return err
		}
	}
	key := revisionStorageKey(revision)
	existingKey := key
	existing, ok := t.state.Revisions[key]
	if !ok && revision.ContractID != "" {
		legacy, legacyOK := t.state.Revisions[revision.ID]
		if legacyOK && legacy.ContractID == "" {
			existing = legacy
			existingKey = revision.ID
			ok = true
		}
	}
	if ok {
		equal, err := immutableRecordsEqual(existing, revision)
		if err != nil {
			return err
		}
		if !equal {
			if !canEnrichLegacyRevisionEvidence(existing, revision) {
				return fmt.Errorf("revision %q conflicts with immutable persisted evidence", revision.ID)
			}
			cloned, err := cloneImmutableRecord(revision)
			if err != nil {
				return err
			}
			if existingKey != key {
				delete(t.state.Revisions, existingKey)
				delete(t.mutatedRevisions, existingKey)
			}
			t.state.Revisions[key] = cloned
			t.mutatedRevisions[key] = struct{}{}
		}
		return nil
	}
	cloned, err := cloneImmutableRecord(revision)
	if err != nil {
		return err
	}
	t.state.Revisions[key] = cloned
	t.mutatedRevisions[key] = struct{}{}
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
	for _, identity := range []struct {
		name  string
		value string
	}{
		{"review contract id", review.ContractID},
		{"review baseline revision id", review.BaselineRevisionID},
		{"review candidate revision id", review.CandidateRevisionID},
	} {
		if err := validateCanonicalIdentity(identity.name, identity.value, false); err != nil {
			return err
		}
	}
	if existing, ok := t.state.Reviews[review.ID]; ok {
		equal, err := immutableRecordsEqual(existing, review)
		if err != nil {
			return err
		}
		if !equal {
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
	cloned, err := cloneImmutableRecord(review)
	if err != nil {
		return err
	}
	t.state.Reviews[review.ID] = cloned
	return nil
}

func (t *operationalTransaction) SaveSyncRecord(ctx context.Context, record domain.SyncRecord) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateID(record.ID); err != nil {
		return err
	}
	for _, identity := range []struct {
		name  string
		value string
	}{
		{"sync project id", record.ProjectID},
		{"sync source id", record.SourceID},
		{"sync revision id", record.RevisionID},
		{"sync ref", record.Ref},
	} {
		if err := validateCanonicalIdentity(identity.name, identity.value, true); err != nil {
			return err
		}
	}
	if record.Result == domain.SyncResultSuccess && record.RevisionID != "" {
		if err := t.bindRevisionOwner(record.ProjectID, record.RevisionID, "sync record "+record.ID); err != nil {
			return err
		}
	}
	if existing, ok := t.state.SyncRecords[record.ID]; ok {
		if !domain.SameSyncEvidence(existing, record) {
			return fmt.Errorf("sync record %q conflicts with immutable persisted evidence", record.ID)
		}
		return nil
	}
	t.state.SyncRecords[record.ID] = record
	return nil
}

func (t *operationalTransaction) SyncRecord(ctx context.Context, id string) (domain.SyncRecord, error) {
	if err := ctx.Err(); err != nil {
		return domain.SyncRecord{}, err
	}
	if err := validateID(id); err != nil {
		return domain.SyncRecord{}, err
	}
	record, ok := t.state.SyncRecords[id]
	if !ok {
		return domain.SyncRecord{}, fs.ErrNotExist
	}
	if record.ID != id {
		return domain.SyncRecord{}, fmt.Errorf("sync lookup %q returned transactional id %q", id, record.ID)
	}
	return record, nil
}

func (t *operationalTransaction) saveReleaseAuthorization(
	ctx context.Context,
	authorization domain.ReleaseAuthorization,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := domain.ValidateReleaseAuthorization(authorization); err != nil {
		return err
	}
	if existing, ok := t.state.ReleaseAuthorizations[authorization.ReviewID]; ok {
		equal, err := immutableRecordsEqual(existing, authorization)
		if err != nil {
			return err
		}
		if !equal {
			return fmt.Errorf(
				"release authorization %q conflicts with immutable persisted evidence",
				authorization.ReviewID,
			)
		}
		return nil
	}
	t.state.ReleaseAuthorizations[authorization.ReviewID] = authorization
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
	if exists && reflect.DeepEqual(current, track) {
		return nil
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
	for _, identity := range []struct {
		name  string
		value string
	}{
		{"audit contract id", event.ContractID},
		{"audit track id", event.TrackID},
		{"audit revision id", event.RevisionID},
		{"audit actor id", event.ActorID},
	} {
		if err := validateCanonicalIdentity(identity.name, identity.value, true); err != nil {
			return err
		}
	}
	if event.RevisionID != "" {
		if err := t.bindRevisionOwner(event.ContractID, event.RevisionID, "audit event "+event.ID); err != nil {
			return err
		}
	}
	if existing, ok := t.state.AuditEvents[event.ID]; ok {
		equal, err := immutableRecordsEqual(existing, event)
		if err != nil {
			return err
		}
		if !equal {
			return fmt.Errorf("audit event %q conflicts with immutable persisted evidence", event.ID)
		}
		return nil
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
	for _, identity := range []struct {
		name  string
		value string
	}{
		{"outbox contract id", message.ContractID},
		{"outbox track id", message.TrackID},
		{"outbox revision id", message.RevisionID},
	} {
		if err := validateCanonicalIdentity(identity.name, identity.value, true); err != nil {
			return err
		}
	}
	if message.RevisionID != "" {
		if err := t.bindRevisionOwner(message.ContractID, message.RevisionID, "outbox message "+message.ID); err != nil {
			return err
		}
	}
	if existing, ok := t.state.Outbox[message.ID]; ok {
		equal, err := immutableRecordsEqual(existing, message)
		if err != nil {
			return err
		}
		if !equal {
			return fmt.Errorf("outbox message %q conflicts with immutable persisted evidence", message.ID)
		}
		return nil
	}
	cloned := message
	cloned.Payload = append([]byte(nil), message.Payload...)
	t.state.Outbox[message.ID] = cloned
	return nil
}

func (t *operationalTransaction) bindRevisionOwner(contractID, revisionID, owner string) error {
	changed, err := bindRevisionOwner(&t.state, contractID, revisionID, owner)
	if err != nil {
		return err
	}
	if changed {
		t.mutatedRevisions[revisionKey(contractID, revisionID)] = struct{}{}
		delete(t.mutatedRevisions, revisionID)
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
	if s.ReleaseAuthorizations == nil {
		s.ReleaseAuthorizations = make(map[string]domain.ReleaseAuthorization)
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
	cloned.ReleaseAuthorizations = make(map[string]domain.ReleaseAuthorization, len(state.ReleaseAuthorizations))
	for key, authorization := range state.ReleaseAuthorizations {
		cloned.ReleaseAuthorizations[key] = authorization
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

func validateReleaseEvidenceAuthorities(state operationalState) error {
	for reviewID, authorization := range state.ReleaseAuthorizations {
		if reviewID != authorization.ReviewID {
			return fmt.Errorf(
				"release authorization key %q does not match review id %q",
				reviewID,
				authorization.ReviewID,
			)
		}
		if err := validateReleaseAuthorizationBundle(state, authorization); err != nil {
			return fmt.Errorf("release authorization %q is invalid: %w", reviewID, err)
		}
	}
	for key, track := range state.ReleaseTracks {
		if track.LastDecision == nil {
			continue
		}
		authorization, ok := state.ReleaseAuthorizations[track.LastDecision.ReviewID]
		if !ok {
			return fmt.Errorf(
				"release track %q decision references missing authorization %q",
				key,
				track.LastDecision.ReviewID,
			)
		}
		if authorization.ContractID != track.ContractID || authorization.TrackID != track.ID {
			return fmt.Errorf("release track %q decision authorization belongs to another track", key)
		}
		if authorization.BoundRef != track.BoundRef {
			return fmt.Errorf("release track %q decision authorization ref does not match bound ref", key)
		}
		if err := validateReleaseDecisionBundle(state, track, authorization); err != nil {
			return fmt.Errorf("release track %q has invalid decision authority: %w", key, err)
		}
	}
	return nil
}

func validateReleaseAuthorizationBundle(
	state operationalState,
	authorization domain.ReleaseAuthorization,
) error {
	if err := domain.ValidateReleaseAuthorization(authorization); err != nil {
		return err
	}
	track, ok := state.ReleaseTracks[releaseTrackKey(authorization.ContractID, authorization.TrackID)]
	if !ok {
		return fmt.Errorf("references uncommitted release track")
	}
	if track.BoundRef != authorization.BoundRef {
		return fmt.Errorf("bound ref does not match release track")
	}
	baseline, ok := revisionForContract(state, authorization.ContractID, authorization.BaselineRevisionID)
	if !ok || baseline.ContractID != authorization.ContractID {
		return fmt.Errorf("baseline revision is missing or belongs to another contract")
	}
	candidate, ok := revisionForContract(state, authorization.ContractID, authorization.CandidateRevisionID)
	if !ok || candidate.ContractID != authorization.ContractID {
		return fmt.Errorf("candidate revision is missing or belongs to another contract")
	}
	if candidate.SourceID != authorization.SourceID || candidate.Ref != authorization.BoundRef {
		return fmt.Errorf("candidate revision source/ref does not match authorization")
	}
	if baseline.ReviewSnapshot == nil || candidate.ReviewSnapshot == nil {
		return fmt.Errorf("authorized revisions require canonical review snapshots")
	}
	review, ok := state.Reviews[authorization.ReviewID]
	if !ok {
		return fmt.Errorf("references uncommitted review")
	}
	if review.ID != authorization.ReviewID ||
		review.ContractID != authorization.ContractID ||
		review.BaselineRevisionID != authorization.BaselineRevisionID ||
		review.CandidateRevisionID != authorization.CandidateRevisionID ||
		review.BaselineSpecDigest != baseline.SpecDigest ||
		review.BaselineContractDigest != baseline.ContractDigest ||
		review.CandidateSpecDigest != candidate.SpecDigest ||
		review.CandidateContractDigest != candidate.ContractDigest ||
		review.Report.PolicyDigest != authorization.PolicyDigest {
		return fmt.Errorf("persisted review does not match authorization and revisions")
	}
	if err := domain.ValidateReleaseReviewReportAgainstSnapshots(
		review.Report,
		authorization.ContractID,
		*baseline.ReviewSnapshot,
		*candidate.ReviewSnapshot,
	); err != nil {
		return fmt.Errorf("canonical review validation: %w", err)
	}
	syncRecord, ok := state.SyncRecords[authorization.SyncRecordID]
	if !ok {
		return fmt.Errorf("references uncommitted sync")
	}
	if syncRecord.ID != authorization.SyncRecordID ||
		syncRecord.ProjectID != authorization.ContractID ||
		syncRecord.SourceID != authorization.SourceID ||
		syncRecord.RevisionID != authorization.CandidateRevisionID ||
		syncRecord.Ref != authorization.BoundRef {
		return fmt.Errorf("persisted sync does not match authorization")
	}
	return nil
}

func validateReleaseDecisionBundle(
	state operationalState,
	track domain.ReleaseTrack,
	authorization domain.ReleaseAuthorization,
) error {
	decision := *track.LastDecision
	review := state.Reviews[authorization.ReviewID]
	canonical, err := domain.CanonicalReviewJSON(review.Report)
	if err != nil {
		return fmt.Errorf("canonical review digest: %w", err)
	}
	sum := sha256.Sum256(canonical)
	if decision.RevisionID != authorization.CandidateRevisionID ||
		decision.ReviewID != authorization.ReviewID ||
		decision.ReviewDigest != hex.EncodeToString(sum[:]) ||
		decision.Verdict != review.Report.Verdict ||
		!decision.EvaluatedAt.Equal(review.Report.EvaluatedAt) {
		return fmt.Errorf("decision identity does not match immutable review")
	}
	if decision.Accepted && state.SyncRecords[authorization.SyncRecordID].Result != domain.SyncResultSuccess {
		return fmt.Errorf("accepted decision requires successful authorized sync")
	}
	promoted := track.Mode == domain.ReleaseModePinned &&
		track.CandidateRevisionID == "" &&
		track.CurrentRevisionID == decision.RevisionID
	decisionGeneration := track.Generation
	if promoted {
		if decisionGeneration == 0 {
			return fmt.Errorf("promoted decision has no consideration generation")
		}
		decisionGeneration--
	}
	if err := requireReleaseTransitionEffects(
		state,
		authorization,
		decision.Accepted,
		decisionGeneration,
		"release.track.considered",
		"release.track.updated",
	); err != nil {
		return err
	}
	if decision.Accepted && (track.Mode == domain.ReleaseModeFollowing || promoted) {
		publication, ok := state.Publications[publicationKey(authorization.ContractID, authorization.CandidateRevisionID)]
		if !ok ||
			!publication.Public ||
			publication.Path != authorization.PublicPath ||
			publication.ProjectID != authorization.ContractID ||
			publication.RevisionID != authorization.CandidateRevisionID {
			return fmt.Errorf("accepted published decision is missing its authorized publication")
		}
	}
	if promoted {
		if err := requireReleaseTransitionEffects(
			state,
			authorization,
			decision.Accepted,
			track.Generation,
			"release.track.promoted",
			"release.track.promoted",
		); err != nil {
			return err
		}
	}
	return nil
}

func requireReleaseTransitionEffects(
	state operationalState,
	authorization domain.ReleaseAuthorization,
	accepted bool,
	generation uint64,
	auditKind, outboxTopic string,
) error {
	eventID := releaseTransitionEvidenceID(authorization, accepted, generation, auditKind)
	event, ok := state.AuditEvents[eventID]
	if !ok ||
		event.ContractID != authorization.ContractID ||
		event.TrackID != authorization.TrackID ||
		event.RevisionID != authorization.CandidateRevisionID ||
		event.Kind != auditKind ||
		event.OccurredAt.IsZero() ||
		event.OccurredAt.Location() != time.UTC {
		return fmt.Errorf("release transition is missing deterministic audit event %q", eventID)
	}
	messageID := "outbox-" + strings.TrimPrefix(eventID, "audit-")
	message, ok := state.Outbox[messageID]
	if !ok ||
		message.ContractID != authorization.ContractID ||
		message.TrackID != authorization.TrackID ||
		message.RevisionID != authorization.CandidateRevisionID ||
		message.Topic != outboxTopic ||
		message.CreatedAt.IsZero() ||
		message.CreatedAt.Location() != time.UTC ||
		!message.CreatedAt.Equal(event.OccurredAt) {
		return fmt.Errorf("release transition is missing deterministic outbox message %q", messageID)
	}
	return nil
}

func releaseTransitionEvidenceID(
	authorization domain.ReleaseAuthorization,
	accepted bool,
	generation uint64,
	kind string,
) string {
	value := fmt.Sprintf(
		"%s\x00%s\x00%s\x00%s\x00%d\x00%t",
		authorization.ContractID,
		authorization.TrackID,
		authorization.CandidateRevisionID,
		authorization.ReviewID,
		generation,
		accepted,
	)
	if kind != "release.track.considered" {
		value += "\x00" + kind
	}
	sum := sha256.Sum256([]byte(value))
	return "audit-" + hex.EncodeToString(sum[:])[:24]
}

func migrateOperationalStateToCurrent(state *operationalState) error {
	switch state.Version {
	case legacyOperationalStateVersion, decisionOperationalStateVersion:
		state.ReleaseTrackAuthorities = make(map[string]releaseTrackAuthority, len(state.ReleaseTracks))
		for key, track := range state.ReleaseTracks {
			// Pre-v3 decision metadata was not cryptographically bound to a
			// persisted review/sync/track authorization. Preserve only the
			// provable last-known-good current revision and generation.
			track.CandidateRevisionID = ""
			track.LastDecision = nil
			state.ReleaseTracks[key] = track
			if err := domain.ValidateReleaseTrack(track); err != nil {
				return fmt.Errorf("migrate legacy release track %q: %w", key, err)
			}
			state.ReleaseTrackAuthorities[key] = newReleaseTrackAuthority(track)
		}
		state.ReleaseAuthorizations = make(map[string]domain.ReleaseAuthorization)
	case authenticatedStateVersion:
		// Authenticated v3 state already has durable review/decision authority.
		// Preserve it byte-for-byte while only changing revision key scope.
	default:
		return fmt.Errorf("cannot migrate operational state version %d", state.Version)
	}
	if err := bindLegacyOperationalRevisionOwners(state); err != nil {
		return fmt.Errorf("migrate legacy revision ownership: %w", err)
	}
	if err := scopeOperationalRevisions(state); err != nil {
		return fmt.Errorf("migrate contract-scoped revisions: %w", err)
	}
	state.Version = operationalStateVersion
	return nil
}

func scopeOperationalRevisions(state *operationalState) error {
	scoped := make(map[string]domain.ContractRevision, len(state.Revisions))
	for _, revision := range state.Revisions {
		key := revisionStorageKey(revision)
		if existing, ok := scoped[key]; ok {
			equal, err := immutableRecordsEqual(existing, revision)
			if err != nil {
				return err
			}
			if !equal {
				return fmt.Errorf("revision %q has conflicting records for contract %q", revision.ID, revision.ContractID)
			}
			continue
		}
		scoped[key] = revision
	}
	state.Revisions = scoped
	return nil
}

func bindLegacyOperationalRevisionOwners(state *operationalState) error {
	for key, track := range state.ReleaseTracks {
		for _, reference := range []struct {
			role       string
			revisionID string
		}{
			{"current", track.CurrentRevisionID},
			{"candidate", track.CandidateRevisionID},
		} {
			if reference.revisionID == "" {
				continue
			}
			if _, err := bindRevisionOwner(
				state,
				track.ContractID,
				reference.revisionID,
				"release track "+key+" "+reference.role,
			); err != nil {
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
	key := revisionKey(contractID, revisionID)
	if revision, ok := state.Revisions[key]; ok {
		if revision.ContractID != contractID || revision.ID != revisionID {
			return false, fmt.Errorf("%s references malformed contract-scoped revision %q", owner, revisionID)
		}
		return false, nil
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
	delete(state.Revisions, revisionID)
	state.Revisions[key] = revision
	return true, nil
}

func validateOperationalReferences(state operationalState) error {
	if err := validatePersistedOperationalIdentities(state); err != nil {
		return err
	}
	for key, revision := range state.Revisions {
		if key != revisionStorageKey(revision) {
			return fmt.Errorf("revision key %q does not match revision identity", key)
		}
	}
	for key, track := range state.ReleaseTracks {
		if key != releaseTrackKey(track.ContractID, track.ID) {
			return fmt.Errorf("release track key %q does not match its identity", key)
		}
		for _, reference := range []struct {
			role       string
			revisionID string
		}{
			{"current", track.CurrentRevisionID},
			{"candidate", track.CandidateRevisionID},
		} {
			if reference.revisionID == "" {
				continue
			}
			if err := requireOwnedRevision(
				state,
				track.ContractID,
				reference.revisionID,
				"release track "+key+" "+reference.role,
			); err != nil {
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
	return validateReleaseEvidenceAuthorities(state)
}

func validatePersistedOperationalIdentities(state operationalState) error {
	for _, revision := range state.Revisions {
		if err := validateID(revision.ID); err != nil {
			return fmt.Errorf("persisted revision id: %w", err)
		}
		for _, identity := range []struct {
			name  string
			value string
		}{
			{"persisted revision contract id", revision.ContractID},
			{"persisted revision source id", revision.SourceID},
			{"persisted revision ref", revision.Ref},
		} {
			if err := validateCanonicalIdentity(identity.name, identity.value, true); err != nil {
				return err
			}
		}
	}
	for _, review := range state.Reviews {
		if err := validateID(review.ID); err != nil {
			return fmt.Errorf("persisted review id: %w", err)
		}
		for _, identity := range []struct {
			name  string
			value string
		}{
			{"persisted review contract id", review.ContractID},
			{"persisted review baseline revision id", review.BaselineRevisionID},
			{"persisted review candidate revision id", review.CandidateRevisionID},
		} {
			if err := validateCanonicalIdentity(identity.name, identity.value, false); err != nil {
				return err
			}
		}
		if containsControlCharacter(review.Report.ContractID) || containsControlCharacter(review.Report.EngineVersion) {
			return fmt.Errorf("persisted review report identity must not contain control characters")
		}
	}
	for _, record := range state.SyncRecords {
		if err := validateID(record.ID); err != nil {
			return fmt.Errorf("persisted sync id: %w", err)
		}
		for _, identity := range []struct {
			name  string
			value string
		}{
			{"persisted sync project id", record.ProjectID},
			{"persisted sync source id", record.SourceID},
			{"persisted sync revision id", record.RevisionID},
			{"persisted sync ref", record.Ref},
		} {
			if err := validateCanonicalIdentity(identity.name, identity.value, true); err != nil {
				return err
			}
		}
	}
	for _, publication := range state.Publications {
		if err := validateID(publication.ProjectID); err != nil {
			return fmt.Errorf("persisted publication contract id: %w", err)
		}
		if err := validateID(publication.RevisionID); err != nil {
			return fmt.Errorf("persisted publication revision id: %w", err)
		}
		if err := validatePublicPath(publication.Path); err != nil {
			return fmt.Errorf("persisted publication path: %w", err)
		}
	}
	for _, event := range state.AuditEvents {
		if err := validateID(event.ID); err != nil {
			return fmt.Errorf("persisted audit id: %w", err)
		}
		for _, identity := range []struct {
			name  string
			value string
		}{
			{"persisted audit contract id", event.ContractID},
			{"persisted audit track id", event.TrackID},
			{"persisted audit revision id", event.RevisionID},
			{"persisted audit actor id", event.ActorID},
		} {
			if err := validateCanonicalIdentity(identity.name, identity.value, true); err != nil {
				return err
			}
		}
	}
	for _, message := range state.Outbox {
		if err := validateID(message.ID); err != nil {
			return fmt.Errorf("persisted outbox id: %w", err)
		}
		for _, identity := range []struct {
			name  string
			value string
		}{
			{"persisted outbox contract id", message.ContractID},
			{"persisted outbox track id", message.TrackID},
			{"persisted outbox revision id", message.RevisionID},
		} {
			if err := validateCanonicalIdentity(identity.name, identity.value, true); err != nil {
				return err
			}
		}
	}
	return nil
}

func requireOwnedRevision(state operationalState, contractID, revisionID, owner string) error {
	if strings.TrimSpace(contractID) == "" {
		return fmt.Errorf("%s has no owning contract", owner)
	}
	revision, ok := revisionForContract(state, contractID, revisionID)
	if !ok {
		return fmt.Errorf("%s references uncommitted revision %q", owner, revisionID)
	}
	if revision.ContractID != contractID {
		return fmt.Errorf("%s references revision %q owned by contract %q, not %q", owner, revisionID, revision.ContractID, contractID)
	}
	return nil
}

func revisionKey(contractID, revisionID string) string {
	if contractID == "" {
		return revisionID
	}
	return contractID + "\x00" + revisionID
}

func revisionStorageKey(revision domain.ContractRevision) string {
	return revisionKey(revision.ContractID, revision.ID)
}

func revisionForContract(
	state operationalState,
	contractID, revisionID string,
) (domain.ContractRevision, bool) {
	revision, ok := state.Revisions[revisionKey(contractID, revisionID)]
	if !ok {
		return domain.ContractRevision{}, false
	}
	if revision.ContractID != contractID || revision.ID != revisionID {
		return domain.ContractRevision{}, false
	}
	return revision, true
}

func revisionsByID(state operationalState, revisionID string) []domain.ContractRevision {
	var revisions []domain.ContractRevision
	for _, revision := range state.Revisions {
		if revision.ID == revisionID {
			revisions = append(revisions, revision)
		}
	}
	sort.Slice(revisions, func(i, j int) bool {
		return revisions[i].ContractID < revisions[j].ContractID
	})
	return revisions
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

func durableAtomicWrite(filePath string, data []byte, mode fs.FileMode) error {
	return durableAtomicWriteWithConfirmation(filePath, data, mode, confirmAtomicReplacement)
}

func durableAtomicWriteWithConfirmation(
	filePath string,
	data []byte,
	mode fs.FileMode,
	confirmReplacement func(string) error,
) error {
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(filePath), atomicWriteStagingPattern)
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
	if err := atomicReplaceFile(temporaryPath, filePath); err != nil {
		return err
	}
	removeTemporary = false
	if confirmReplacement == nil {
		confirmReplacement = confirmAtomicReplacement
	}
	if err := confirmReplacement(filepath.Dir(filePath)); err != nil {
		return fmt.Errorf(
			"%w: confirm atomic replacement of %q: %v",
			port.ErrCommitOutcomeUnknown,
			filePath,
			err,
		)
	}
	return nil
}

func validateID(id string) error {
	if id != strings.TrimSpace(id) {
		return fmt.Errorf("identity must not contain leading or trailing whitespace")
	}
	if id == "" || id == "." || id == ".." || filepath.IsAbs(id) {
		return errUnsafeStorePath
	}
	if strings.ContainsAny(id, `/\`) || filepath.Clean(id) != id {
		return errUnsafeStorePath
	}
	if containsControlCharacter(id) {
		return fmt.Errorf("identity must not contain control characters")
	}
	return nil
}

func validateCanonicalIdentity(name, value string, allowEmpty bool) error {
	if value == "" {
		if allowEmpty {
			return nil
		}
		return fmt.Errorf("%s is required", name)
	}
	if value != strings.TrimSpace(value) {
		return fmt.Errorf("%s must not contain leading or trailing whitespace", name)
	}
	if containsControlCharacter(value) {
		return fmt.Errorf("%s must not contain control characters", name)
	}
	return nil
}

func immutableRecordsEqual(left, right any) (bool, error) {
	leftJSON, err := json.Marshal(left)
	if err != nil {
		return false, fmt.Errorf("encode persisted immutable record: %w", err)
	}
	rightJSON, err := json.Marshal(right)
	if err != nil {
		return false, fmt.Errorf("encode replayed immutable record: %w", err)
	}
	return bytes.Equal(leftJSON, rightJSON), nil
}

func cloneImmutableRecord[T any](value T) (T, error) {
	var cloned T
	encoded, err := json.Marshal(value)
	if err != nil {
		return cloned, fmt.Errorf("encode immutable record clone: %w", err)
	}
	if err := json.Unmarshal(encoded, &cloned); err != nil {
		return cloned, fmt.Errorf("decode immutable record clone: %w", err)
	}
	return cloned, nil
}

func validatePublicPath(publicPath string) error {
	if publicPath == "" ||
		publicPath != strings.TrimSpace(publicPath) ||
		!strings.HasPrefix(publicPath, "/") ||
		strings.Contains(publicPath, `\`) ||
		containsControlCharacter(publicPath) {
		return errUnsafeStorePath
	}
	if path.Clean(publicPath) != publicPath {
		return errUnsafeStorePath
	}
	return nil
}

func containsControlCharacter(value string) bool {
	for _, character := range value {
		if unicode.IsControl(character) {
			return true
		}
	}
	return false
}

var _ port.UnitOfWork = (*FileStore)(nil)
var _ port.BlobStore = (*FileStore)(nil)
var _ port.RevisionReader = (*FileStore)(nil)
var _ port.OperationalStore = (*operationalTransaction)(nil)
