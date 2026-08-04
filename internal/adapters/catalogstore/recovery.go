package catalogstore

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/araihu/manja/application/catalog"
)

func (coordinator *ActivationCoordinator) recover(ctx context.Context) error {
	if err := coordinator.removeIncompleteStaging(); err != nil {
		return err
	}
	if err := coordinator.restoreDurableRoutes(ctx); err != nil {
		return err
	}
	journalBytes, err := os.ReadFile(coordinator.journalPath())
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("catalogstore: read activation journal: %w", err)
	}
	var journal activationJournalV1
	if err := decodeStrict(journalBytes, &journal); err != nil || journal.SchemaVersion != 1 {
		return fmt.Errorf("%w: invalid activation journal: %v", ErrCorruptSnapshot, err)
	}
	table := coordinator.runtime.Table()
	if state, exists := table.Mounts[journal.Mount]; exists && state.Active.ID == journal.Candidate {
		return coordinator.removeJournal()
	}
	current := catalog.SnapshotID("")
	if state, exists := table.Mounts[journal.Mount]; exists {
		current = state.Active.ID
	}
	if table.Generation != journal.Generation || current != journal.ExpectedOld {
		if err := coordinator.archiveJournal(journalBytes); err != nil {
			return fmt.Errorf("catalogstore: archive stale journal: %w", err)
		}
		return nil
	}
	verified, err := coordinator.store.Preflight(ctx, journal.Candidate)
	if err != nil {
		return err
	}
	if _, err := coordinator.runtime.ActivateMountDurablyBounded(journal.Mount, journal.ExpectedOld, journal.Generation, verified, coordinator.writeRouteTable); err != nil {
		return fmt.Errorf("catalogstore: recover activation: %w", err)
	}
	return coordinator.removeJournal()
}

func (coordinator *ActivationCoordinator) restoreDurableRoutes(ctx context.Context) error {
	data, err := os.ReadFile(coordinator.routeTablePath())
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("catalogstore: read durable routes: %w", err)
	}
	var persisted durableRouteTableV1
	if err := decodeStrict(data, &persisted); err != nil || persisted.SchemaVersion != 1 {
		return fmt.Errorf("%w: invalid durable routes: %v", ErrCorruptSnapshot, err)
	}
	table := &catalog.RouteTable{Generation: persisted.Generation, Mounts: make(map[string]catalog.MountState, len(persisted.Mounts))}
	repaired := false
	for mount, entry := range persisted.Mounts {
		active, err := coordinator.store.Preflight(ctx, entry.Active)
		if err != nil {
			if !errors.Is(err, ErrCorruptSnapshot) {
				return fmt.Errorf("catalogstore: preflight active mount %q: %w", mount, err)
			}
			repaired = true
			if quarantineErr := coordinator.quarantine(entry.Active); quarantineErr != nil {
				return quarantineErr
			}
			if entry.Previous == "" {
				continue
			}
			active, err = coordinator.store.Preflight(ctx, entry.Previous)
			if err != nil {
				if errors.Is(err, ErrCorruptSnapshot) {
					if quarantineErr := coordinator.quarantine(entry.Previous); quarantineErr != nil {
						return quarantineErr
					}
					continue
				}
				return fmt.Errorf("catalogstore: preflight fallback mount %q: %w", mount, err)
			}
			table.Mounts[mount] = catalog.MountState{Active: active}
			continue
		}
		state := catalog.MountState{Active: active}
		if entry.Previous != "" {
			previous, err := coordinator.store.Preflight(ctx, entry.Previous)
			if err != nil {
				if !errors.Is(err, ErrCorruptSnapshot) {
					return fmt.Errorf("catalogstore: preflight previous mount %q: %w", mount, err)
				}
				repaired = true
				if quarantineErr := coordinator.quarantine(entry.Previous); quarantineErr != nil {
					return quarantineErr
				}
			} else {
				state.Previous = &previous
			}
		}
		table.Mounts[mount] = state
	}
	if repaired {
		if err := coordinator.writeRouteTable(table); err != nil {
			return fmt.Errorf("catalogstore: persist recovered fallback routes: %w", err)
		}
	}
	if err := coordinator.runtime.RestoreTable(table); err != nil {
		return fmt.Errorf("catalogstore: restore runtime routes: %w", err)
	}
	return nil
}

func (coordinator *ActivationCoordinator) removeIncompleteStaging() error {
	stagingRoot := filepath.Join(coordinator.store.root, "staging")
	entries, err := os.ReadDir(stagingRoot)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("catalogstore: inspect staging: %w", err)
	}
	for _, entry := range entries {
		if err := os.RemoveAll(filepath.Join(stagingRoot, entry.Name())); err != nil {
			return fmt.Errorf("catalogstore: remove incomplete staging %q: %w", entry.Name(), err)
		}
	}
	return nil
}
