package team

import (
	"os"
	"os/exec"
	"testing"
)

func TestProjectKey(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)

	mustGit := func(args ...string) {
		t.Helper()
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	mustGit("init")
	mustGit("remote", "add", "origin", "git@github.com:Acme/App.git")

	got, err := ProjectKey(dir)
	if err != nil {
		t.Fatalf("ProjectKey: %v", err)
	}
	if want := "github.com/acme/app"; got != want {
		t.Errorf("ProjectKey = %q, want %q", got, want)
	}
}

func TestProjectKeyNoRemote(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
	if out, err := exec.Command("git", "-C", dir, "init").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	if _, err := ProjectKey(dir); err == nil {
		t.Error("expected an error for a repo with no origin remote")
	}
}

func TestProjectKeyEmptyDir(t *testing.T) {
	if _, err := ProjectKey(""); err == nil {
		t.Error("expected an error for an empty project dir")
	}
}
