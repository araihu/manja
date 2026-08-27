package catalogstore

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/araihu/manja/application/catalog"
	storeprimitives "github.com/araihu/manja/internal/adapters/store"
)

func (coordinator *ActivationCoordinator) GarbageCollect(ctx context.Context) error {
	coordinator.commit.Lock()
	defer coordinator.commit.Unlock()
	return coordinator.garbageCollectLocked(ctx)
}

func (coordinator *ActivationCoordinator) garbageCollectLocked(ctx context.Context) error {
	keep := make(map[catalog.SnapshotID]struct{})
	for _, state := range coordinator.runtime.Table().Mounts {
		keep[state.Active.ID] = struct{}{}
		if state.Previous != nil {
			keep[state.Previous.ID] = struct{}{}
		}
	}
	root := filepath.Join(coordinator.store.root, "snapshots")
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("catalogstore: inspect snapshots for garbage collection: %w", err)
	}
	changed := false
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		id := catalog.SnapshotID(entry.Name())
		if _, retained := keep[id]; retained || coordinator.runtime.ReferenceCount(id) != 0 {
			continue
		}
		if err := os.RemoveAll(filepath.Join(root, entry.Name())); err != nil {
			return fmt.Errorf("catalogstore: remove unreferenced snapshot %q: %w", id, err)
		}
		changed = true
	}
	if changed {
		if err := storeprimitives.SyncDirectory(root); err != nil {
			return fmt.Errorf("catalogstore: confirm garbage collection: %w", err)
		}
	}
	return nil
}

func (coordinator *ActivationCoordinator) HandleCorruption(ctx context.Context, mount string, snapshotID catalog.SnapshotID) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	coordinator.commit.Lock()
	defer coordinator.commit.Unlock()
	table := coordinator.runtime.Table()
	state, exists := table.Mounts[mount]
	if !exists || state.Active.ID != snapshotID {
		return fmt.Errorf("%w: mount %q no longer serves %q", catalog.ErrStaleSnapshot, mount, snapshotID)
	}
	if err := coordinator.rejectPendingJournal(); err != nil {
		return err
	}
	if state.Previous != nil {
		if _, err := coordinator.store.Preflight(ctx, state.Previous.ID); err != nil {
			if !errors.Is(err, ErrCorruptSnapshot) {
				return err
			}
			// Remove the mount durably before quarantine. A failed quarantine
			// must never leave an unverified previous generation serving.
			if _, disableErr := coordinator.runtime.DisableMountDurably(mount, snapshotID, table.Generation, coordinator.writeRouteTable); disableErr != nil {
				return disableErr
			}
			if err := coordinator.quarantine(state.Previous.ID); err != nil {
				return err
			}
		} else {
			if _, err := coordinator.runtime.FallbackMountDurably(mount, snapshotID, table.Generation, coordinator.writeRouteTable); err != nil {
				return err
			}
		}
	} else if _, err := coordinator.runtime.DisableMountDurably(mount, snapshotID, table.Generation, coordinator.writeRouteTable); err != nil {
		return err
	}
	if err := coordinator.quarantine(snapshotID); err != nil {
		return err
	}
	return nil
}

func (coordinator *ActivationCoordinator) quarantine(id catalog.SnapshotID) error {
	source, err := coordinator.store.snapshotPath(id)
	if err != nil {
		return err
	}
	destination := filepath.Join(coordinator.store.root, "quarantine", string(id))
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return fmt.Errorf("catalogstore: create quarantine: %w", err)
	}
	if err := os.Rename(source, destination); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("catalogstore: quarantine %q: %w", id, err)
	}
	for _, directory := range []string{filepath.Dir(source), filepath.Dir(destination)} {
		if err := storeprimitives.SyncDirectory(directory); err != nil {
			return fmt.Errorf("catalogstore: confirm quarantine %q: %w", id, err)
		}
	}
	return nil
}
