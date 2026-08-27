//go:build !aix && !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris && !windows

package source

import (
	"os"
	"os/exec"
	"time"
)

const gitCommandWaitDelay = 2 * time.Second

func prepareGitProcess(command *exec.Cmd) {
	command.Cancel = func() error { return killGitProcessTree(command) }
	command.WaitDelay = gitCommandWaitDelay
}

func killGitProcessTree(command *exec.Cmd) error {
	if command.Process == nil {
		return os.ErrProcessDone
	}
	return command.Process.Kill()
}
