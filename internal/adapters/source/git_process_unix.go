//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package source

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
	"time"
)

const gitCommandWaitDelay = 500 * time.Millisecond

func prepareGitProcess(command *exec.Cmd) {
	if command.SysProcAttr == nil {
		command.SysProcAttr = &syscall.SysProcAttr{}
	}
	command.SysProcAttr.Setpgid = true
	command.Cancel = func() error { return killGitProcessTree(command) }
	command.WaitDelay = gitCommandWaitDelay
}

func killGitProcessTree(command *exec.Cmd) error {
	if command.Process == nil {
		return os.ErrProcessDone
	}
	err := syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
	if err == nil || errors.Is(err, syscall.ESRCH) {
		return nil
	}
	return command.Process.Kill()
}
