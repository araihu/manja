//go:build windows

package store

import "golang.org/x/sys/windows"

func atomicReplaceFile(source, destination string) error {
	sourcePath, err := windows.UTF16PtrFromString(source)
	if err != nil {
		return err
	}
	destinationPath, err := windows.UTF16PtrFromString(destination)
	if err != nil {
		return err
	}
	return windows.MoveFileEx(
		sourcePath,
		destinationPath,
		windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH,
	)
}

// MoveFileEx with MOVEFILE_WRITE_THROUGH does not return until the move has
// completed on disk. Windows directory handles are therefore neither opened
// nor flushed by the common writer.
func confirmAtomicReplacement(string) error {
	return nil
}
