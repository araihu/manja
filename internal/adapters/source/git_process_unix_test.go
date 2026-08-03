//go:build darwin || linux

package source

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestRunGitCommandCancellationKillsInheritedPipeDescendant(t *testing.T) {
	pidFile := t.TempDir() + "/child.pid"
	ctx, cancel := context.WithCancel(context.Background())
	command := exec.CommandContext(ctx, "sh", "-c", `sleep 30 & child=$!; printf '%s' "$child" > "$MANJA_CHILD_PID_FILE"; wait "$child"`)
	command.Env = append(os.Environ(), "MANJA_CHILD_PID_FILE="+pidFile)
	command.Stdout = &bytes.Buffer{}
	result := make(chan error, 1)
	go func() { result <- runGitCommand(ctx, command, "") }()

	childPID := waitForChildPID(t, pidFile)
	t.Cleanup(func() { _ = syscall.Kill(childPID, syscall.SIGKILL) })
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) && !strings.Contains(err.Error(), "signal: killed") {
			t.Fatalf("canceled command error = %v", err)
		}
	case <-time.After(750 * time.Millisecond):
		_ = syscall.Kill(childPID, syscall.SIGKILL)
		<-result
		t.Fatal("canceled command remained blocked on descendant-owned pipe")
	}
	if processExists(childPID) {
		t.Fatalf("descendant process %d survived cancellation", childPID)
	}
}

func waitForChildPID(t *testing.T, path string) int {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		if err == nil {
			pid, parseErr := strconv.Atoi(string(data))
			if parseErr != nil {
				t.Fatalf("parse child PID %q: %v", data, parseErr)
			}
			return pid
		}
		if !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("descendant PID was not written")
	return 0
}

func processExists(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}
