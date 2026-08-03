//go:build windows

package source

import (
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"time"
)

const gitCommandWaitDelay = 2 * time.Second

func prepareGitProcess(command *exec.Cmd) {
	if command.SysProcAttr == nil {
		command.SysProcAttr = &syscall.SysProcAttr{}
	}
	command.SysProcAttr.CreationFlags |= syscall.CREATE_NEW_PROCESS_GROUP
	command.Cancel = func() error { return killGitProcessTree(command) }
	command.WaitDelay = gitCommandWaitDelay
}

func killGitProcessTree(command *exec.Cmd) error {
	if command.Process == nil {
		return os.ErrProcessDone
	}
	if err := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(command.Process.Pid)).Run(); err == nil {
		return nil
	}
	return command.Process.Kill()
}
