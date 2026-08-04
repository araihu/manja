package store

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// DurableAtomicWrite exposes the repository's platform-aware atomic file
// replacement for immutable catalog journals and route pointers.
func DurableAtomicWrite(filePath string, data []byte, mode fs.FileMode) error {
	return durableAtomicWrite(filePath, data, mode)
}

// DurableRenameNew publishes an already-synced staging directory at a new,
// immutable destination and confirms the parent-directory update.
func DurableRenameNew(source, destination string) error {
	if _, err := os.Lstat(destination); err == nil {
		return fs.ErrExist
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.Rename(source, destination); err != nil {
		return err
	}
	if err := confirmAtomicReplacement(filepath.Dir(destination)); err != nil {
		return fmt.Errorf("confirm immutable directory publication: %w", err)
	}
	return nil
}

func SyncDirectory(directoryPath string) error {
	return confirmAtomicReplacement(directoryPath)
}

type ExclusiveFileLock struct {
	file *os.File
	once sync.Once
}

func AcquireExclusiveFileLock(ctx context.Context, filePath string) (*ExclusiveFileLock, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	for {
		acquired, lockErr := tryOperationalFileLock(file)
		if lockErr != nil {
			_ = file.Close()
			return nil, lockErr
		}
		if acquired {
			return &ExclusiveFileLock{file: file}, nil
		}
		timer := time.NewTimer(10 * time.Millisecond)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			_ = file.Close()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

func (lock *ExclusiveFileLock) Release() error {
	if lock == nil || lock.file == nil {
		return nil
	}
	var result error
	lock.once.Do(func() {
		if err := unlockOperationalFile(lock.file); err != nil {
			result = err
		}
		if err := lock.file.Close(); result == nil {
			result = err
		}
	})
	return result
}
