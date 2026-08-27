package catalogstore

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/araihu/manja/application/catalog"
)

const (
	MaxSnapshotBytes = uint64(64 << 20)
	MaxStagingBytes  = uint64(128 << 20)
	MaxStoredBytes   = uint64(512 << 20)
)

var (
	ErrCorruptSnapshot   = errors.New("catalogstore: corrupt snapshot")
	ErrTransientSnapshot = errors.New("catalogstore: snapshot temporarily unavailable")
	ErrStorageBudget     = errors.New("catalogstore: storage budget exceeded")
)

type Store struct {
	root           string
	resourceLimits bool
}

type Materialization struct {
	ID       catalog.SnapshotID
	Location string
	Bytes    uint64
}

func New(root string) *Store {
	return NewWithResourceLimits(root, true)
}

func NewWithResourceLimits(root string, resourceLimits bool) *Store {
	return &Store{root: filepath.Clean(root), resourceLimits: resourceLimits}
}

func (store *Store) Root() string {
	return store.root
}

func (store *Store) snapshotPath(id catalog.SnapshotID) (string, error) {
	if !validSnapshotID(id) {
		return "", fmt.Errorf("%w: invalid snapshot ID %q", ErrCorruptSnapshot, id)
	}
	return filepath.Join(store.root, "snapshots", string(id)), nil
}

func safeChildPath(root, child string) (string, error) {
	if child == "" || strings.HasPrefix(child, "/") || strings.Contains(child, `\`) || filepath.Clean(child) != child || strings.HasPrefix(child, ".."+string(filepath.Separator)) || child == ".." {
		return "", fmt.Errorf("%w: unsafe child path %q", ErrCorruptSnapshot, child)
	}
	joined := filepath.Join(root, filepath.FromSlash(child))
	relative, err := filepath.Rel(root, joined)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: unsafe child path %q", ErrCorruptSnapshot, child)
	}
	return joined, nil
}

func validSnapshotID(id catalog.SnapshotID) bool {
	const prefix = "snapshot-sha256-"
	text := string(id)
	digest := strings.TrimPrefix(text, prefix)
	if !strings.HasPrefix(text, prefix) || len(digest) != 64 {
		return false
	}
	for _, character := range digest {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

func directoryBytes(root string) (uint64, error) {
	var total uint64
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if entry.Type().IsRegular() {
			info, infoErr := entry.Info()
			if infoErr != nil {
				return infoErr
			}
			total += uint64(info.Size())
		}
		return nil
	})
	if os.IsNotExist(err) {
		return 0, nil
	}
	return total, err
}
