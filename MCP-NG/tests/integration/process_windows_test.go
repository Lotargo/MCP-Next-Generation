//go:build windows

package integration

import (
	"os/exec"
)

func setupProcessGroup(cmd *exec.Cmd) {
	// Windows doesn't support Setpgid in the same way.
	// We rely on basic execution.
}

func killProcessGroup(cmd *exec.Cmd) {
	if cmd.Process != nil {
		// On Windows, Kill() terminates the process.
		// It avoids the compilation error caused by syscall.SIGKILL.
		cmd.Process.Kill()
	}
}
