//go:build windows

package team

import "os/exec"

// detach is a no-op on Windows (native Windows is out of scope; this keeps the
// package building for the untested archives).
func detach(cmd *exec.Cmd) {}
