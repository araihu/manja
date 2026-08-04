//go:build darwin || linux

package source

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

var errMalformedChildPID = errors.New("malformed child PID")

func TestRunGitCommandCancellationKillsInheritedPipeDescendant(t *testing.T) {
	pidFile := t.TempDir() + "/child.pid"
	ctx, cancel := context.WithCancel(context.Background())
	command := exec.CommandContext(ctx, "sh", "-c", `sleep 30 & child=$!; printf '%s\n' "$child" > "$MANJA_CHILD_PID_FILE"; wait "$child"`)
	command.Env = append(os.Environ(), "MANJA_CHILD_PID_FILE="+pidFile)
	command.Stdout = &bytes.Buffer{}
	result := make(chan error, 1)
	go func() { result <- runGitCommand(ctx, command, "") }()

	childPID, err := waitForChildPID(pidFile, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
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

func TestWaitForChildPIDRetriesEmptyFileUntilPIDIsWritten(t *testing.T) {
	pidFile := t.TempDir() + "/child.pid"
	if err := os.WriteFile(pidFile, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	writeResult := make(chan error, 1)
	go func() {
		time.Sleep(25 * time.Millisecond)
		if err := os.WriteFile(pidFile, []byte("42"), 0o600); err != nil {
			writeResult <- err
			return
		}
		time.Sleep(25 * time.Millisecond)
		writeResult <- os.WriteFile(pidFile, []byte("4242\n"), 0o600)
	}()

	pid, err := waitForChildPID(pidFile, 500*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if pid != 4242 {
		t.Fatalf("child PID = %d, want 4242", pid)
	}
	if err := <-writeResult; err != nil {
		t.Fatal(err)
	}
}

func TestWaitForChildPIDRejectsNonemptyMalformedContent(t *testing.T) {
	pidFile := t.TempDir() + "/child.pid"
	if err := os.WriteFile(pidFile, []byte("not-a-pid"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := waitForChildPID(pidFile, 500*time.Millisecond); !errors.Is(err, errMalformedChildPID) {
		t.Fatalf("malformed PID error = %v, want %v", err, errMalformedChildPID)
	}
}

func waitForChildPID(path string, timeout time.Duration) (int, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		if err == nil {
			if len(data) == 0 {
				time.Sleep(5 * time.Millisecond)
				continue
			}
			complete := data[len(data)-1] == '\n'
			value := data
			if complete {
				value = data[:len(data)-1]
			}
			for _, character := range value {
				if character < '0' || character > '9' {
					return 0, fmt.Errorf("%w: %q", errMalformedChildPID, data)
				}
			}
			if !complete || len(value) == 0 {
				time.Sleep(5 * time.Millisecond)
				continue
			}
			pid, parseErr := strconv.Atoi(string(value))
			if parseErr != nil {
				return 0, fmt.Errorf("%w: %q: %v", errMalformedChildPID, data, parseErr)
			}
			if pid <= 0 {
				return 0, fmt.Errorf("%w: %q", errMalformedChildPID, data)
			}
			return pid, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return 0, err
		}
		time.Sleep(5 * time.Millisecond)
	}
	return 0, fmt.Errorf("descendant PID was not written within %s", timeout)
}

func processExists(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}
