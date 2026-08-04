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
var errProcessStillRunning = errors.New("process is still running")

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
	if err := waitForProcessNotRunning(childPID, 250*time.Millisecond); err != nil {
		t.Fatalf("descendant process %d survived cancellation: %v", childPID, err)
	}
}

func TestWaitForProcessNotRunningAllowsDelayedExit(t *testing.T) {
	command := exec.Command("sh", "-c", "sleep 0.05")
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()

	if err := waitForProcessNotRunning(command.Process.Pid, 500*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestWaitForProcessNotRunningAcceptsZombie(t *testing.T) {
	command := exec.Command("sh", "-c", "exit 0")
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	if err := waitForProcessNotRunning(command.Process.Pid, 500*time.Millisecond); err != nil {
		_ = command.Process.Kill()
		_, _ = command.Process.Wait()
		t.Fatal(err)
	}
	if _, err := command.Process.Wait(); err != nil {
		t.Fatal(err)
	}
}

func TestWaitForProcessNotRunningRejectsLiveSurvivor(t *testing.T) {
	command := exec.Command("sleep", "30")
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = command.Process.Kill()
		_, _ = command.Process.Wait()
	})

	started := time.Now()
	err := waitForProcessNotRunning(command.Process.Pid, 50*time.Millisecond)
	if !errors.Is(err, errProcessStillRunning) {
		t.Fatalf("live survivor error = %v, want %v", err, errProcessStillRunning)
	}
	if elapsed := time.Since(started); elapsed < 50*time.Millisecond || elapsed > 500*time.Millisecond {
		t.Fatalf("live survivor check elapsed = %s, want bounded timeout near 50ms", elapsed)
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

func waitForProcessNotRunning(pid int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		running, err := processIsRunning(pid)
		if err != nil {
			return err
		}
		if !running {
			return nil
		}
		if !time.Now().Before(deadline) {
			return fmt.Errorf("%w after %s", errProcessStillRunning, timeout)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func processIsRunning(pid int) (bool, error) {
	err := syscall.Kill(pid, 0)
	if errors.Is(err, syscall.ESRCH) {
		return false, nil
	}
	if err != nil && !errors.Is(err, syscall.EPERM) {
		return false, err
	}

	output, psErr := exec.Command("ps", "-o", "stat=", "-p", strconv.Itoa(pid)).Output()
	if psErr != nil {
		// The process can disappear between kill(2) and ps(1).
		err = syscall.Kill(pid, 0)
		if errors.Is(err, syscall.ESRCH) {
			return false, nil
		}
		if err == nil || errors.Is(err, syscall.EPERM) {
			return true, nil
		}
		return false, err
	}
	state := strings.TrimSpace(string(output))
	if state == "" {
		return false, nil
	}
	for _, marker := range state {
		switch marker {
		case 'E', 'X', 'Z':
			return false, nil
		}
	}
	return true, nil
}
