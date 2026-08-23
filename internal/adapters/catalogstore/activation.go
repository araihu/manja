package catalogstore

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/araihu/manja/application/catalog"
	storeprimitives "github.com/araihu/manja/internal/adapters/store"
)

type ActivationReceipt struct {
	Mount      string
	SnapshotID catalog.SnapshotID
	PreviousID catalog.SnapshotID
	Generation uint64
	Location   string
}

type WithdrawalReceipt struct {
	Mount      string
	State      catalog.TombstoneState
	SnapshotID catalog.SnapshotID
	Generation uint64
}

// ActivationAdmission is a context-capturing boundary hook. ActivateAdmitted
// first uses one to quiesce callers after immutable preflight, then another
// after the complete next route table has been built and encoded. Either may
// abort before the durable route pointer or runtime table changes.
type ActivationAdmission func() error

var (
	ErrActivationPending  = errors.New("catalogstore: activation recovery pending")
	ErrActivationIdentity = errors.New("catalogstore: activation identity mismatch")
)

const (
	maxDurableRouteTableBytes     = 1 << 20
	maxActivationJournalBytes     = 16 << 10
	maxDurableMounts              = 64
	maxArchivedActivationJournals = 16
)

type ActivationCoordinator struct {
	store          *Store
	runtime        *catalog.Runtime
	resourceLimits bool
	lock           *storeprimitives.ExclusiveFileLock
	close          sync.Once
	commit         sync.Mutex
	hooks          activationHooks
}

type activationHooks struct {
	afterPreflight func() error
	afterJournal   func() error
	afterPointer   func() error
	afterRuntime   func() error
}

type durableRouteTableV1 struct {
	SchemaVersion uint32                        `json:"schemaVersion"`
	Generation    uint64                        `json:"generation"`
	Mounts        map[string]durableMountV1     `json:"mounts"`
	Tombstones    map[string]durableTombstoneV1 `json:"tombstones,omitempty"`
}

type durableMountV1 struct {
	CatalogID string             `json:"catalogId,omitempty"`
	Active    catalog.SnapshotID `json:"active"`
	Previous  catalog.SnapshotID `json:"previous,omitempty"`
}

type durableTombstoneV1 struct {
	State      catalog.TombstoneState `json:"state"`
	CatalogID  string                 `json:"catalogId"`
	SnapshotID catalog.SnapshotID     `json:"snapshotId"`
}

type activationJournalV1 struct {
	SchemaVersion uint32              `json:"schemaVersion"`
	Mount         string              `json:"mount"`
	CatalogID     string              `json:"catalogId,omitempty"`
	Candidate     catalog.SnapshotID  `json:"candidate"`
	ExpectedOld   catalog.SnapshotID  `json:"expectedOld,omitempty"`
	Generation    uint64              `json:"generation"`
	Operation     string              `json:"operation,omitempty"`
	Tombstone     *durableTombstoneV1 `json:"tombstone,omitempty"`
}

const (
	activationOperationWithdraw    = "withdraw"
	activationOperationReauthorize = "reauthorize"
)

func OpenActivationCoordinator(ctx context.Context, root string, runtime *catalog.Runtime) (*ActivationCoordinator, error) {
	return OpenActivationCoordinatorWithResourceLimits(ctx, root, runtime, true)
}

func OpenActivationCoordinatorWithResourceLimits(ctx context.Context, root string, runtime *catalog.Runtime, resourceLimits bool) (*ActivationCoordinator, error) {
	if runtime == nil {
		return nil, fmt.Errorf("catalogstore: runtime is required")
	}
	store := NewWithResourceLimits(root, resourceLimits)
	lock, err := storeprimitives.AcquireExclusiveFileLock(ctx, filepath.Join(root, ".catalog.lock"))
	if err != nil {
		return nil, fmt.Errorf("catalogstore: acquire data-directory lock: %w", err)
	}
	coordinator := &ActivationCoordinator{store: store, runtime: runtime, lock: lock, resourceLimits: resourceLimits}
	if err := coordinator.recover(ctx); err != nil {
		_ = lock.Release()
		return nil, err
	}
	if err := coordinator.GarbageCollect(ctx); err != nil {
		_ = lock.Release()
		return nil, err
	}
	return coordinator, nil
}

func (coordinator *ActivationCoordinator) Close() error {
	if coordinator == nil {
		return nil
	}
	var result error
	coordinator.close.Do(func() { result = coordinator.lock.Release() })
	return result
}

func (coordinator *ActivationCoordinator) Store() *Store {
	return coordinator.store
}

func (coordinator *ActivationCoordinator) Activate(
	ctx context.Context,
	mount string,
	expectedOld catalog.SnapshotID,
	generation uint64,
	candidate catalog.CompiledSnapshot,
) (ActivationReceipt, error) {
	return coordinator.ActivateAdmitted(ctx, mount, expectedOld, generation, candidate, nil, nil)
}

func (coordinator *ActivationCoordinator) Withdraw(
	ctx context.Context,
	mount string,
	expectedActive catalog.SnapshotID,
	generation uint64,
	state catalog.TombstoneState,
) (WithdrawalReceipt, error) {
	coordinator.commit.Lock()
	defer coordinator.commit.Unlock()
	if err := ctx.Err(); err != nil {
		return WithdrawalReceipt{}, err
	}
	if err := coordinator.rejectPendingJournal(); err != nil {
		return WithdrawalReceipt{}, err
	}
	current := coordinator.runtime.Table()
	active, exists := current.Mounts[mount]
	if !exists {
		return WithdrawalReceipt{}, fmt.Errorf("%w: mount %q has no active snapshot", catalog.ErrStaleSnapshot, mount)
	}
	tombstone := catalog.MountTombstone{State: state, CatalogID: active.Active.Directory.CatalogID, SnapshotID: active.Active.ID}
	persisted := durableTombstone(tombstone)
	journal := activationJournalV1{
		SchemaVersion: 1,
		Mount:         mount,
		CatalogID:     tombstone.CatalogID,
		ExpectedOld:   expectedActive,
		Generation:    generation,
		Operation:     activationOperationWithdraw,
		Tombstone:     &persisted,
	}
	if err := validateActivationJournalSize(journal); err != nil {
		return WithdrawalReceipt{}, err
	}
	if err := coordinator.validateWithdrawalBounds(mount, expectedActive, generation, tombstone); err != nil {
		return WithdrawalReceipt{}, err
	}
	persist := func(table *catalog.RouteTable) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		data, err := coordinator.encodeRouteTable(table)
		if err != nil {
			return err
		}
		if err := coordinator.writeJournal(journal); err != nil {
			return err
		}
		if coordinator.hooks.afterJournal != nil {
			if err := coordinator.hooks.afterJournal(); err != nil {
				return err
			}
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := coordinator.writeRouteTableBytes(data); err != nil {
			return err
		}
		if coordinator.hooks.afterPointer != nil {
			return coordinator.hooks.afterPointer()
		}
		return nil
	}
	if _, err := coordinator.runtime.WithdrawMountDurably(mount, expectedActive, generation, tombstone, persist); err != nil {
		return WithdrawalReceipt{}, err
	}
	if coordinator.hooks.afterRuntime != nil {
		if err := coordinator.hooks.afterRuntime(); err != nil {
			return WithdrawalReceipt{}, err
		}
	}
	if err := coordinator.removeJournal(); err != nil {
		return WithdrawalReceipt{}, err
	}
	return WithdrawalReceipt{Mount: mount, State: state, SnapshotID: tombstone.SnapshotID, Generation: generation}, nil
}

func (coordinator *ActivationCoordinator) Reauthorize(
	ctx context.Context,
	mount string,
	generation uint64,
	candidate catalog.CompiledSnapshot,
) (ActivationReceipt, error) {
	coordinator.commit.Lock()
	defer coordinator.commit.Unlock()
	if err := ctx.Err(); err != nil {
		return ActivationReceipt{}, err
	}
	if err := coordinator.rejectPendingJournal(); err != nil {
		return ActivationReceipt{}, err
	}
	current := coordinator.runtime.Table()
	tombstone, exists := current.Tombstones[mount]
	if !exists {
		return ActivationReceipt{}, fmt.Errorf("%w: mount %q is not tombstoned", catalog.ErrMountUnavailable, mount)
	}
	if candidate.Directory.CatalogID != "" && tombstone.CatalogID != candidate.Directory.CatalogID {
		return ActivationReceipt{}, fmt.Errorf("%w: mount %q catalog changed from %q to %q", ErrActivationIdentity, mount, tombstone.CatalogID, candidate.Directory.CatalogID)
	}
	journal := activationJournalV1{
		SchemaVersion: 1,
		Mount:         mount,
		CatalogID:     candidate.Directory.CatalogID,
		Candidate:     candidate.ID,
		ExpectedOld:   tombstone.SnapshotID,
		Generation:    generation,
		Operation:     activationOperationReauthorize,
	}
	if err := validateActivationJournalSize(journal); err != nil {
		return ActivationReceipt{}, err
	}
	if err := coordinator.validateReauthorizationBounds(mount, generation, candidate); err != nil {
		return ActivationReceipt{}, err
	}
	materialization, err := coordinator.store.Publish(ctx, candidate)
	if errors.Is(err, ErrStorageBudget) {
		if collectErr := coordinator.garbageCollectLocked(ctx); collectErr != nil {
			return ActivationReceipt{}, collectErr
		}
		materialization, err = coordinator.store.Publish(ctx, candidate)
	}
	if err != nil {
		return ActivationReceipt{}, err
	}
	verified, err := coordinator.store.Preflight(ctx, candidate.ID)
	if err != nil {
		return ActivationReceipt{}, err
	}
	if coordinator.hooks.afterPreflight != nil {
		if err := coordinator.hooks.afterPreflight(); err != nil {
			return ActivationReceipt{}, err
		}
	}
	if err := ctx.Err(); err != nil {
		return ActivationReceipt{}, err
	}
	current = coordinator.runtime.Table()
	tombstone, exists = current.Tombstones[mount]
	if !exists {
		return ActivationReceipt{}, fmt.Errorf("%w: mount %q is not tombstoned", catalog.ErrMountUnavailable, mount)
	}
	if tombstone.CatalogID != verified.Directory.CatalogID {
		return ActivationReceipt{}, fmt.Errorf("%w: mount %q catalog changed from %q to %q", ErrActivationIdentity, mount, tombstone.CatalogID, verified.Directory.CatalogID)
	}
	journal = activationJournalV1{
		SchemaVersion: 1,
		Mount:         mount,
		CatalogID:     string(verified.Directory.CatalogID),
		Candidate:     candidate.ID,
		ExpectedOld:   tombstone.SnapshotID,
		Generation:    generation,
		Operation:     activationOperationReauthorize,
	}
	persist := func(table *catalog.RouteTable) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		data, err := coordinator.encodeRouteTable(table)
		if err != nil {
			return err
		}
		if err := coordinator.writeJournal(journal); err != nil {
			return err
		}
		if coordinator.hooks.afterJournal != nil {
			if err := coordinator.hooks.afterJournal(); err != nil {
				return err
			}
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := coordinator.writeRouteTableBytes(data); err != nil {
			return err
		}
		if coordinator.hooks.afterPointer != nil {
			return coordinator.hooks.afterPointer()
		}
		return nil
	}
	if _, err := coordinator.runtime.ReauthorizeMountDurably(mount, generation, verified, persist); err != nil {
		return ActivationReceipt{}, err
	}
	if coordinator.hooks.afterRuntime != nil {
		if err := coordinator.hooks.afterRuntime(); err != nil {
			return ActivationReceipt{}, err
		}
	}
	if err := coordinator.removeJournal(); err != nil {
		return ActivationReceipt{}, err
	}
	return ActivationReceipt{Mount: mount, SnapshotID: candidate.ID, PreviousID: "", Generation: generation, Location: materialization.Location}, nil
}

func (coordinator *ActivationCoordinator) ActivateAdmitted(
	ctx context.Context,
	mount string,
	expectedOld catalog.SnapshotID,
	generation uint64,
	candidate catalog.CompiledSnapshot,
	quiesce ActivationAdmission,
	admit ActivationAdmission,
) (ActivationReceipt, error) {
	coordinator.commit.Lock()
	defer coordinator.commit.Unlock()
	if err := ctx.Err(); err != nil {
		return ActivationReceipt{}, err
	}
	if err := coordinator.rejectPendingJournal(); err != nil {
		return ActivationReceipt{}, err
	}
	journal := activationJournalV1{
		SchemaVersion: 1,
		Mount:         mount,
		CatalogID:     candidate.Directory.CatalogID,
		Candidate:     candidate.ID,
		ExpectedOld:   expectedOld,
		Generation:    generation,
	}
	if err := validateActivationJournalSize(journal); err != nil {
		return ActivationReceipt{}, err
	}
	if err := coordinator.validateActivationBounds(mount, expectedOld, generation, candidate); err != nil {
		return ActivationReceipt{}, err
	}

	materialization, err := coordinator.store.Publish(ctx, candidate)
	if errors.Is(err, ErrStorageBudget) {
		if collectErr := coordinator.garbageCollectLocked(ctx); collectErr != nil {
			return ActivationReceipt{}, collectErr
		}
		materialization, err = coordinator.store.Publish(ctx, candidate)
	}
	if err != nil {
		return ActivationReceipt{}, err
	}
	verified, err := coordinator.store.Preflight(ctx, candidate.ID)
	if err != nil {
		return ActivationReceipt{}, err
	}
	if coordinator.hooks.afterPreflight != nil {
		if err := coordinator.hooks.afterPreflight(); err != nil {
			return ActivationReceipt{}, err
		}
	}
	if err := ctx.Err(); err != nil {
		return ActivationReceipt{}, err
	}
	if err := coordinator.runtime.CheckMount(mount, expectedOld, generation); err != nil {
		return ActivationReceipt{}, err
	}
	if state, exists := coordinator.runtime.Table().Mounts[mount]; exists && state.Active.Directory.CatalogID != verified.Directory.CatalogID {
		return ActivationReceipt{}, fmt.Errorf("%w: mount %q catalog changed from %q to %q", ErrActivationIdentity, mount, state.Active.Directory.CatalogID, verified.Directory.CatalogID)
	}
	// Quiesce before taking Runtime's writer lock. In-flight renderers may need
	// that lock while draining their existing admissions.
	if quiesce != nil {
		if err := quiesce(); err != nil {
			return ActivationReceipt{}, err
		}
	}
	// An unchanged activation has no route-table transition, so its persist
	// callback is intentionally skipped by Runtime. It still needs an admitted
	// post-staging sample before reporting success.
	if candidate.ID == expectedOld && admit != nil {
		if err := admit(); err != nil {
			return ActivationReceipt{}, err
		}
	}
	journal = activationJournalV1{SchemaVersion: 1, Mount: mount, CatalogID: string(verified.Directory.CatalogID), Candidate: candidate.ID, ExpectedOld: expectedOld, Generation: generation}
	persist := func(table *catalog.RouteTable) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		data, err := coordinator.encodeRouteTable(table)
		if err != nil {
			return err
		}
		if admit != nil {
			if err := admit(); err != nil {
				return err
			}
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := coordinator.writeJournal(journal); err != nil {
			return err
		}
		if coordinator.hooks.afterJournal != nil {
			if err := coordinator.hooks.afterJournal(); err != nil {
				return err
			}
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := coordinator.writeRouteTableBytes(data); err != nil {
			return err
		}
		if coordinator.hooks.afterPointer != nil {
			return coordinator.hooks.afterPointer()
		}
		return nil
	}
	activation, err := coordinator.runtime.ActivateMountDurablyBounded(mount, expectedOld, generation, verified, persist)
	if err != nil {
		return ActivationReceipt{}, err
	}
	if coordinator.hooks.afterRuntime != nil {
		if err := coordinator.hooks.afterRuntime(); err != nil {
			return ActivationReceipt{}, err
		}
	}
	if err := coordinator.removeJournal(); err != nil {
		return ActivationReceipt{}, err
	}
	return ActivationReceipt{Mount: mount, SnapshotID: candidate.ID, PreviousID: activation.PreviousID, Generation: activation.Generation, Location: materialization.Location}, nil
}

func (coordinator *ActivationCoordinator) writeRouteTable(table *catalog.RouteTable) error {
	data, err := coordinator.encodeRouteTable(table)
	if err != nil {
		return err
	}
	return coordinator.writeRouteTableBytes(data)
}

func (coordinator *ActivationCoordinator) validateActivationBounds(mount string, expectedOld catalog.SnapshotID, generation uint64, candidate catalog.CompiledSnapshot) error {
	if err := coordinator.runtime.CheckMount(mount, expectedOld, generation); err != nil {
		return err
	}
	if candidate.ID == expectedOld {
		return nil
	}
	current := coordinator.runtime.Table()
	state, exists := current.Mounts[mount]
	next := catalog.MountState{Active: catalog.RuntimeSnapshot{
		ID: candidate.ID,
		Directory: catalog.CatalogArtifactV1{
			CatalogID: candidate.Directory.CatalogID,
		},
	}}
	if exists {
		previous := state.Active
		next.Previous = &previous
	}
	current.Mounts[mount] = next
	_, err := coordinator.encodeRouteTable(current)
	return err
}

func (coordinator *ActivationCoordinator) validateWithdrawalBounds(mount string, expectedActive catalog.SnapshotID, generation uint64, tombstone catalog.MountTombstone) error {
	if err := coordinator.runtime.CheckMount(mount, expectedActive, generation); err != nil {
		return err
	}
	current := coordinator.runtime.Table()
	delete(current.Mounts, mount)
	current.Tombstones[mount] = tombstone
	_, err := coordinator.encodeRouteTable(current)
	return err
}

func (coordinator *ActivationCoordinator) validateReauthorizationBounds(mount string, generation uint64, candidate catalog.CompiledSnapshot) error {
	current := coordinator.runtime.Table()
	if current.Generation != generation {
		return fmt.Errorf("%w: expected %d, current %d", catalog.ErrStaleGeneration, generation, current.Generation)
	}
	if _, exists := current.Tombstones[mount]; !exists {
		return fmt.Errorf("%w: mount %q is not tombstoned", catalog.ErrMountUnavailable, mount)
	}
	delete(current.Tombstones, mount)
	current.Mounts[mount] = catalog.MountState{Active: catalog.RuntimeSnapshot{
		ID: candidate.ID,
		Directory: catalog.CatalogArtifactV1{
			CatalogID: candidate.Directory.CatalogID,
		},
	}}
	_, err := coordinator.encodeRouteTable(current)
	return err
}

func encodeRouteTable(table *catalog.RouteTable) ([]byte, error) {
	return encodeRouteTableWithResourceLimits(table, true)
}

func (coordinator *ActivationCoordinator) encodeRouteTable(table *catalog.RouteTable) ([]byte, error) {
	return encodeRouteTableWithResourceLimits(table, coordinator.resourceLimits)
}

func encodeRouteTableWithResourceLimits(table *catalog.RouteTable, resourceLimits bool) ([]byte, error) {
	if table == nil {
		return nil, fmt.Errorf("%w: route table is nil", ErrStorageBudget)
	}
	if resourceLimits && len(table.Mounts)+len(table.Tombstones) > maxDurableMounts {
		return nil, fmt.Errorf("%w: route mounts exceed %d", ErrStorageBudget, maxDurableMounts)
	}
	persisted := durableRouteTableV1{SchemaVersion: 1, Generation: table.Generation, Mounts: make(map[string]durableMountV1, len(table.Mounts)), Tombstones: make(map[string]durableTombstoneV1, len(table.Tombstones))}
	for mount, state := range table.Mounts {
		entry := durableMountV1{CatalogID: state.Active.Directory.CatalogID, Active: state.Active.ID}
		if state.Previous != nil {
			entry.Previous = state.Previous.ID
		}
		persisted.Mounts[mount] = entry
	}
	for mount, tombstone := range table.Tombstones {
		persisted.Tombstones[mount] = durableTombstone(tombstone)
	}
	data, err := json.Marshal(persisted)
	if err != nil {
		return nil, fmt.Errorf("catalogstore: encode route table: %w", err)
	}
	if resourceLimits && len(data) > maxDurableRouteTableBytes {
		return nil, fmt.Errorf("%w: route table is %d bytes, maximum is %d", ErrStorageBudget, len(data), maxDurableRouteTableBytes)
	}
	return data, nil
}

func durableTombstone(value catalog.MountTombstone) durableTombstoneV1 {
	return durableTombstoneV1{State: value.State, CatalogID: value.CatalogID, SnapshotID: value.SnapshotID}
}

func runtimeTombstone(value durableTombstoneV1) catalog.MountTombstone {
	return catalog.MountTombstone{State: value.State, CatalogID: value.CatalogID, SnapshotID: value.SnapshotID}
}

func (coordinator *ActivationCoordinator) writeRouteTableBytes(data []byte) error {
	if coordinator.resourceLimits && len(data) > maxDurableRouteTableBytes {
		return fmt.Errorf("%w: route table is %d bytes, maximum is %d", ErrStorageBudget, len(data), maxDurableRouteTableBytes)
	}
	if err := storeprimitives.DurableAtomicWrite(coordinator.routeTablePath(), data, 0o600); err != nil {
		return fmt.Errorf("catalogstore: persist route table: %w", err)
	}
	return nil
}

func (coordinator *ActivationCoordinator) writeJournal(journal activationJournalV1) error {
	data, err := encodeActivationJournal(journal)
	if err != nil {
		return err
	}
	if err := storeprimitives.DurableAtomicWrite(coordinator.journalPath(), data, 0o600); err != nil {
		return fmt.Errorf("catalogstore: persist activation journal: %w", err)
	}
	return nil
}

func encodeActivationJournal(journal activationJournalV1) ([]byte, error) {
	data, err := json.Marshal(journal)
	if err != nil {
		return nil, fmt.Errorf("catalogstore: encode activation journal: %w", err)
	}
	if len(data) > maxActivationJournalBytes {
		return nil, fmt.Errorf("%w: activation journal is %d bytes, maximum is %d", ErrStorageBudget, len(data), maxActivationJournalBytes)
	}
	return data, nil
}

func validateActivationJournalSize(journal activationJournalV1) error {
	_, err := encodeActivationJournal(journal)
	return err
}

func (coordinator *ActivationCoordinator) removeJournal() error {
	err := os.Remove(coordinator.journalPath())
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("catalogstore: remove activation journal: %w", err)
	}
	if err == nil {
		if syncErr := storeprimitives.SyncDirectory(filepath.Dir(coordinator.journalPath())); syncErr != nil {
			return fmt.Errorf("catalogstore: confirm journal removal: %w", syncErr)
		}
	}
	return nil
}

func (coordinator *ActivationCoordinator) routeTablePath() string {
	return filepath.Join(coordinator.store.root, "state", "routes.json")
}

func (coordinator *ActivationCoordinator) journalPath() string {
	return filepath.Join(coordinator.store.root, "state", "activation-journal.json")
}

func decodeStrict[T any](data []byte, result *T) error {
	if len(data) == 0 || !utf8.Valid(data) {
		return fmt.Errorf("invalid UTF-8 or empty JSON")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	if err := decoder.Decode(result); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return fmt.Errorf("trailing JSON data")
	}
	canonical, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("re-encode JSON: %w", err)
	}
	if !bytes.Equal(canonical, data) {
		return fmt.Errorf("non-canonical JSON")
	}
	return nil
}

func (coordinator *ActivationCoordinator) archiveJournal(data []byte) error {
	directory := filepath.Join(coordinator.store.root, "state", "stale-journals")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	digest := sha256.Sum256(data)
	name := fmt.Sprintf("%d-%x.json", time.Now().UTC().UnixNano(), digest[:8])
	if err := storeprimitives.DurableAtomicWrite(filepath.Join(directory, name), data, 0o600); err != nil {
		return err
	}
	if err := coordinator.trimArchivedJournals(directory); err != nil {
		return err
	}
	return coordinator.removeJournal()
}

func (coordinator *ActivationCoordinator) rejectPendingJournal() error {
	_, err := os.Stat(coordinator.journalPath())
	switch {
	case err == nil:
		return ErrActivationPending
	case os.IsNotExist(err):
		return nil
	default:
		return fmt.Errorf("catalogstore: inspect activation journal: %w", err)
	}
}

func (coordinator *ActivationCoordinator) trimArchivedJournals(directory string) error {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return err
	}
	if len(entries) <= maxArchivedActivationJournals {
		return nil
	}
	// ReadDir returns lexical order. Names contain a UTC nanosecond prefix, so
	// the oldest archived entries sort first. Ignore non-JSON files rather than
	// treating unrelated operator files as activation journals.
	files := make([]os.DirEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		files = append(files, entry)
	}
	sort.Slice(files, func(left, right int) bool { return files[left].Name() < files[right].Name() })
	remove := len(files) - maxArchivedActivationJournals
	changed := false
	for index := 0; index < remove; index++ {
		if err := os.Remove(filepath.Join(directory, files[index].Name())); err != nil {
			return err
		}
		changed = true
	}
	if changed {
		return storeprimitives.SyncDirectory(directory)
	}
	return nil
}
