//go:build !windows

package store

import "os"

func atomicReplaceFile(source, destination string) error {
	return os.Rename(source, destination)
}

func confirmAtomicReplacement(directoryPath string) error {
	directory, err := os.Open(directoryPath)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
