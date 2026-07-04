package team

import (
	"os/exec"
	"testing"
)

func TestHasGit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	if !HasGit() {
		t.Error("HasGit should be true when git is on PATH")
	}
	t.Setenv("PATH", "") // hide every executable, including git
	if HasGit() {
		t.Error("HasGit should be false with an empty PATH")
	}
}
