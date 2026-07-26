package architecture_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestFileStoreCompilesForWindows(t *testing.T) {
	output := filepath.Join(t.TempDir(), "store.test.exe")
	cmd := exec.Command("go", "test", "-c", "-o", output, "./internal/adapters/store")
	cmd.Dir = repositoryRoot(t)
	cmd.Env = append(os.Environ(), "GOOS=windows", "GOARCH=amd64", "CGO_ENABLED=0")
	combined, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("cross-compile filesystem store for windows: %v\n%s", err, combined)
	}
}
