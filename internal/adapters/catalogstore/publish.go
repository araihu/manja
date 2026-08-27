package catalogstore

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/araihu/manja/application/catalog"
	storeprimitives "github.com/araihu/manja/internal/adapters/store"
)

func (store *Store) Publish(ctx context.Context, snapshot catalog.CompiledSnapshot) (Materialization, error) {
	if err := ctx.Err(); err != nil {
		return Materialization{}, err
	}
	finalPath, err := store.snapshotPath(snapshot.ID)
	if err != nil {
		return Materialization{}, err
	}
	children := append([]catalog.ChildArtifact(nil), snapshot.Children...)
	sort.Slice(children, func(left, right int) bool { return children[left].Path < children[right].Path })
	var total uint64
	for index, child := range children {
		if index > 0 && children[index-1].Path == child.Path {
			return Materialization{}, fmt.Errorf("%w: duplicate child %q", ErrCorruptSnapshot, child.Path)
		}
		if _, err := safeChildPath(finalPath, child.Path); err != nil {
			return Materialization{}, err
		}
		digest := sha256.Sum256(child.Bytes)
		if child.Length != uint64(len(child.Bytes)) || child.SHA256 != hex.EncodeToString(digest[:]) || child.Length == 0 {
			return Materialization{}, fmt.Errorf("%w: child %q bytes differ from identity", ErrCorruptSnapshot, child.Path)
		}
		total += child.Length
		if store.resourceLimits && total > MaxSnapshotBytes {
			return Materialization{}, fmt.Errorf("%w: snapshot is %d bytes", ErrStorageBudget, total)
		}
	}
	if len(children) == 0 {
		return Materialization{}, fmt.Errorf("%w: snapshot has no children", ErrCorruptSnapshot)
	}
	if info, statErr := os.Stat(finalPath); statErr == nil && info.IsDir() {
		verified, preflightErr := store.Preflight(ctx, snapshot.ID)
		if preflightErr != nil {
			return Materialization{}, preflightErr
		}
		return Materialization{ID: snapshot.ID, Location: verified.Location, Bytes: total}, nil
	} else if statErr != nil && !os.IsNotExist(statErr) {
		return Materialization{}, fmt.Errorf("%w: inspect immutable destination: %v", ErrTransientSnapshot, statErr)
	}

	stagingRoot := filepath.Join(store.root, "staging")
	stagingBytes, err := directoryBytes(stagingRoot)
	if err != nil {
		return Materialization{}, fmt.Errorf("%w: inspect staging: %v", ErrTransientSnapshot, err)
	}
	storedBytes, err := directoryBytes(filepath.Join(store.root, "snapshots"))
	if err != nil {
		return Materialization{}, fmt.Errorf("%w: inspect snapshots: %v", ErrTransientSnapshot, err)
	}
	if store.resourceLimits && (stagingBytes+total > MaxStagingBytes || storedBytes+total > MaxStoredBytes) {
		return Materialization{}, fmt.Errorf("%w: staging=%d stored=%d candidate=%d", ErrStorageBudget, stagingBytes, storedBytes, total)
	}
	if err := os.MkdirAll(stagingRoot, 0o755); err != nil {
		return Materialization{}, fmt.Errorf("%w: create staging: %v", ErrTransientSnapshot, err)
	}
	stagingPath, err := os.MkdirTemp(stagingRoot, ".snapshot-")
	if err != nil {
		return Materialization{}, fmt.Errorf("%w: create staging candidate: %v", ErrTransientSnapshot, err)
	}
	removeStaging := true
	defer func() {
		if removeStaging {
			_ = os.RemoveAll(stagingPath)
		}
	}()
	for _, child := range children {
		if err := ctx.Err(); err != nil {
			return Materialization{}, err
		}
		path, pathErr := safeChildPath(stagingPath, child.Path)
		if pathErr != nil {
			return Materialization{}, pathErr
		}
		if err := writeImmutableFile(path, child.Bytes); err != nil {
			return Materialization{}, fmt.Errorf("%w: write child %q: %v", ErrTransientSnapshot, child.Path, err)
		}
	}
	if err := syncTreeDirectories(stagingPath); err != nil {
		return Materialization{}, fmt.Errorf("%w: sync staging: %v", ErrTransientSnapshot, err)
	}
	if err := os.MkdirAll(filepath.Dir(finalPath), 0o755); err != nil {
		return Materialization{}, fmt.Errorf("%w: create snapshots directory: %v", ErrTransientSnapshot, err)
	}
	if err := storeprimitives.DurableRenameNew(stagingPath, finalPath); err != nil {
		return Materialization{}, fmt.Errorf("%w: publish immutable snapshot: %v", ErrTransientSnapshot, err)
	}
	removeStaging = false
	return Materialization{ID: snapshot.ID, Location: finalPath, Bytes: total}, nil
}

func writeImmutableFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o444)
	if err != nil {
		return err
	}
	closeWith := func(input error) error {
		if closeErr := file.Close(); input == nil {
			return closeErr
		}
		return input
	}
	if _, err := io.Copy(file, bytes.NewReader(data)); err != nil {
		return closeWith(err)
	}
	if err := file.Sync(); err != nil {
		return closeWith(err)
	}
	return closeWith(nil)
}

func syncTreeDirectories(root string) error {
	var directories []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			directories = append(directories, path)
		}
		return nil
	})
	if err != nil {
		return err
	}
	sort.Slice(directories, func(left, right int) bool { return len(directories[left]) > len(directories[right]) })
	for _, directory := range directories {
		if err := storeprimitives.SyncDirectory(directory); err != nil {
			return err
		}
	}
	return nil
}
