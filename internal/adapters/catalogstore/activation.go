package catalogstore

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

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

type ActivationCoordinator struct {
	store   *Store
	runtime *catalog.Runtime
	lock    *storeprimitives.ExclusiveFileLock
	close   sync.Once
	commit  sync.Mutex
	hooks   activationHooks
}

type activationHooks struct {
	afterPreflight func() error
	afterJournal   func() error
	afterPointer   func() error
	afterRuntime   func() error
}

type durableRouteTableV1 struct {
	SchemaVersion uint32                    `json:"schemaVersion"`
	Generation    uint64                    `json:"generation"`
	Mounts        map[string]durableMountV1 `json:"mounts"`
}

type durableMountV1 struct {
	Active   catalog.SnapshotID `json:"active"`
	Previous catalog.SnapshotID `json:"previous,omitempty"`
}

type activationJournalV1 struct {
	SchemaVersion uint32             `json:"schemaVersion"`
	Mount         string             `json:"mount"`
	Candidate     catalog.SnapshotID `json:"candidate"`
	ExpectedOld   catalog.SnapshotID `json:"expectedOld,omitempty"`
	Generation    uint64             `json:"generation"`
}

func OpenActivationCoordinator(ctx context.Context, root string, runtime *catalog.Runtime) (*ActivationCoordinator, error) {
	if runtime == nil {
		return nil, fmt.Errorf("catalogstore: runtime is required")
	}
	store := New(root)
	lock, err := storeprimitives.AcquireExclusiveFileLock(ctx, filepath.Join(root, ".catalog.lock"))
	if err != nil {
		return nil, fmt.Errorf("catalogstore: acquire data-directory lock: %w", err)
	}
	coordinator := &ActivationCoordinator{store: store, runtime: runtime, lock: lock}
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
	materialization, err := coordinator.store.Publish(ctx, candidate)
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
	journal := activationJournalV1{SchemaVersion: 1, Mount: mount, Candidate: candidate.ID, ExpectedOld: expectedOld, Generation: generation}
	coordinator.commit.Lock()
	defer coordinator.commit.Unlock()
	if err := coordinator.runtime.CheckMount(mount, expectedOld, generation); err != nil {
		return ActivationReceipt{}, err
	}
	if err := coordinator.writeJournal(journal); err != nil {
		return ActivationReceipt{}, err
	}
	if coordinator.hooks.afterJournal != nil {
		if err := coordinator.hooks.afterJournal(); err != nil {
			return ActivationReceipt{}, err
		}
	}
	persist := func(table *catalog.RouteTable) error {
		if err := coordinator.writeRouteTable(table); err != nil {
			return err
		}
		if coordinator.hooks.afterPointer != nil {
			return coordinator.hooks.afterPointer()
		}
		return nil
	}
	next, err := coordinator.runtime.ActivateMountDurably(mount, expectedOld, generation, verified, persist)
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
	state := next.Mounts[mount]
	previousID := catalog.SnapshotID("")
	if state.Previous != nil {
		previousID = state.Previous.ID
	}
	return ActivationReceipt{Mount: mount, SnapshotID: candidate.ID, PreviousID: previousID, Generation: next.Generation, Location: materialization.Location}, nil
}

func (coordinator *ActivationCoordinator) writeRouteTable(table *catalog.RouteTable) error {
	persisted := durableRouteTableV1{SchemaVersion: 1, Generation: table.Generation, Mounts: make(map[string]durableMountV1, len(table.Mounts))}
	for mount, state := range table.Mounts {
		entry := durableMountV1{Active: state.Active.ID}
		if state.Previous != nil {
			entry.Previous = state.Previous.ID
		}
		persisted.Mounts[mount] = entry
	}
	data, err := json.Marshal(persisted)
	if err != nil {
		return fmt.Errorf("catalogstore: encode route table: %w", err)
	}
	if err := storeprimitives.DurableAtomicWrite(coordinator.routeTablePath(), data, 0o600); err != nil {
		return fmt.Errorf("catalogstore: persist route table: %w", err)
	}
	return nil
}

func (coordinator *ActivationCoordinator) writeJournal(journal activationJournalV1) error {
	data, err := json.Marshal(journal)
	if err != nil {
		return fmt.Errorf("catalogstore: encode activation journal: %w", err)
	}
	if err := storeprimitives.DurableAtomicWrite(coordinator.journalPath(), data, 0o600); err != nil {
		return fmt.Errorf("catalogstore: persist activation journal: %w", err)
	}
	return nil
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
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(result); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return fmt.Errorf("trailing JSON data")
	}
	return nil
}

func (coordinator *ActivationCoordinator) archiveJournal(data []byte) error {
	directory := filepath.Join(coordinator.store.root, "state", "stale-journals")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	name := fmt.Sprintf("%d.json", time.Now().UTC().UnixNano())
	if err := storeprimitives.DurableAtomicWrite(filepath.Join(directory, name), data, 0o600); err != nil {
		return err
	}
	return coordinator.removeJournal()
}
