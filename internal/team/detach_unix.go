//go:build !windows

package team

import (
	"os/exec"
	"syscall"
)

// detach starts cmd in its own session, with no controlling terminal. ssh
// opens /dev/tty directly to ask for a key passphrase or to confirm an unknown
// host key — GIT_TERMINAL_PROMPT does not reach it — and from inside the TUI
// that prompt would land on the screen Bubble Tea owns. Without a terminal it
// fails instead (read as unreachable), while keys in ssh-agent keep working.
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
