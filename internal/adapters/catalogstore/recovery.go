package catalogstore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/araihu/manja/application/catalog"
	"github.com/araihu/manja/domain"
)

func (coordinator *ActivationCoordinator) recover(ctx context.Context) error {
	if err := coordinator.removeIncompleteStaging(); err != nil {
		return err
	}
	if err := coordinator.restoreDurableRoutes(ctx); err != nil {
		return err
	}
	journalBytes, err := readDurableState(coordinator.journalPath(), maxActivationJournalBytes)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("%w: read activation journal: %v", ErrCorruptSnapshot, err)
	}
	var journal activationJournalV1
	if err := decodeStrict(journalBytes, &journal); err != nil || journal.SchemaVersion != 1 {
		return fmt.Errorf("%w: invalid activation journal: %v", ErrCorruptSnapshot, err)
	}
	if err := validateActivationJournal(journal); err != nil {
		return fmt.Errorf("%w: invalid activation journal: %v", ErrCorruptSnapshot, err)
	}
	table := coordinator.runtime.Table()
	if state, exists := table.Mounts[journal.Mount]; exists && state.Active.ID == journal.Candidate {
		if journal.CatalogID != "" && state.Active.Directory.CatalogID != journal.CatalogID {
			return fmt.Errorf("%w: activation journal catalog %q differs from active catalog %q", ErrCorruptSnapshot, journal.CatalogID, state.Active.Directory.CatalogID)
		}
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
	if journal.CatalogID != "" && verified.Directory.CatalogID != journal.CatalogID {
		return fmt.Errorf("%w: activation candidate catalog %q differs from journal catalog %q", ErrCorruptSnapshot, verified.Directory.CatalogID, journal.CatalogID)
	}
	if state, exists := table.Mounts[journal.Mount]; exists && state.Active.ID == journal.ExpectedOld && state.Active.Directory.CatalogID != verified.Directory.CatalogID {
		return fmt.Errorf("%w: activation candidate catalog %q differs from active catalog %q", ErrCorruptSnapshot, verified.Directory.CatalogID, state.Active.Directory.CatalogID)
	}
	if _, err := coordinator.runtime.ActivateMountDurablyBounded(journal.Mount, journal.ExpectedOld, journal.Generation, verified, coordinator.writeRouteTable); err != nil {
		return fmt.Errorf("catalogstore: recover activation: %w", err)
	}
	return coordinator.removeJournal()
}

func (coordinator *ActivationCoordinator) restoreDurableRoutes(ctx context.Context) error {
	data, err := readDurableState(coordinator.routeTablePath(), maxDurableRouteTableBytes)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("%w: read durable routes: %v", ErrCorruptSnapshot, err)
	}
	var persisted durableRouteTableV1
	if err := decodeStrict(data, &persisted); err != nil || persisted.SchemaVersion != 1 {
		return fmt.Errorf("%w: invalid durable routes: %v", ErrCorruptSnapshot, err)
	}
	if err := validateDurableRouteTable(persisted); err != nil {
		return fmt.Errorf("%w: invalid durable routes: %v", ErrCorruptSnapshot, err)
	}
	table := &catalog.RouteTable{Generation: persisted.Generation, Mounts: make(map[string]catalog.MountState, len(persisted.Mounts))}
	repaired := false
	for mount, entry := range persisted.Mounts {
		active, err := coordinator.store.Preflight(ctx, entry.Active)
		if err == nil && entry.CatalogID != "" && active.Directory.CatalogID != entry.CatalogID {
			err = fmt.Errorf("%w: active catalog %q differs from route catalog %q", ErrCorruptSnapshot, active.Directory.CatalogID, entry.CatalogID)
		}
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
			if entry.CatalogID == "" {
				// A legacy route table has no durable catalog binding. Keep the
				// mount disabled rather than guessing that an unverified previous
				// snapshot belongs to the same catalog as the corrupt active one.
				repaired = true
				continue
			}
			if entry.CatalogID != "" && active.Directory.CatalogID != entry.CatalogID {
				repaired = true
				if quarantineErr := coordinator.quarantine(entry.Previous); quarantineErr != nil {
					return quarantineErr
				}
				continue
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
				if (entry.CatalogID != "" && previous.Directory.CatalogID != entry.CatalogID) || previous.Directory.CatalogID != active.Directory.CatalogID {
					repaired = true
					if quarantineErr := coordinator.quarantine(entry.Previous); quarantineErr != nil {
						return quarantineErr
					}
					// Active is already verified. Drop only the mismatched
					// previous generation; never turn a healthy route into a
					// missing mount because rollback metadata is corrupt.
				} else {
					state.Previous = &previous
				}
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
		return fmt.Errorf("%w: restore runtime routes: %v", ErrCorruptSnapshot, err)
	}
	return nil
}

func readDurableState(path string, limit int) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, int64(limit)+1))
	if err != nil {
		return nil, err
	}
	if len(data) > limit {
		return nil, fmt.Errorf("state exceeds %d bytes", limit)
	}
	return data, nil
}

func validateActivationJournal(journal activationJournalV1) error {
	if !validActivationMount(journal.Mount) {
		return fmt.Errorf("mount %q is invalid", journal.Mount)
	}
	if !validSnapshotID(journal.Candidate) {
		return fmt.Errorf("candidate snapshot %q is invalid", journal.Candidate)
	}
	if journal.ExpectedOld != "" && !validSnapshotID(journal.ExpectedOld) {
		return fmt.Errorf("expected old snapshot %q is invalid", journal.ExpectedOld)
	}
	if journal.CatalogID != "" {
		if err := domain.ValidateCatalogID(journal.CatalogID); err != nil {
			return fmt.Errorf("catalog identity: %w", err)
		}
	}
	return nil
}

func validateDurableRouteTable(table durableRouteTableV1) error {
	if table.Mounts == nil {
		return fmt.Errorf("mounts are missing")
	}
	if len(table.Mounts) > maxDurableMounts {
		return fmt.Errorf("mounts exceed %d", maxDurableMounts)
	}
	for mount, entry := range table.Mounts {
		if !validActivationMount(mount) {
			return fmt.Errorf("mount %q is invalid", mount)
		}
		if !validSnapshotID(entry.Active) {
			return fmt.Errorf("active snapshot %q is invalid", entry.Active)
		}
		if entry.Previous != "" {
			if !validSnapshotID(entry.Previous) {
				return fmt.Errorf("previous snapshot %q is invalid", entry.Previous)
			}
			if entry.Previous == entry.Active {
				return fmt.Errorf("active and previous snapshots are identical")
			}
		}
		if entry.CatalogID != "" {
			if err := domain.ValidateCatalogID(entry.CatalogID); err != nil {
				return fmt.Errorf("catalog identity: %w", err)
			}
		}
	}
	return nil
}

func validActivationMount(value string) bool {
	if value == "/" {
		return true
	}
	if value == "" || !strings.HasPrefix(value, "/") || strings.HasSuffix(value, "/") || strings.ContainsAny(value, `\\?#`) || path.Clean(value) != value {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
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
