//go:build !windows

package client

import (
	"os/exec"
	"syscall"
)

// detach starts the service in its own session, outliving the caller.
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
