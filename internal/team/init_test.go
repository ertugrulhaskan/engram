package team

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// hermeticGitEnv points Dir() inside root and isolates git from the machine's
// global/system config (identity via env vars, no gpg signing, no stray defaults)
// so the tests behave the same everywhere.
func hermeticGitEnv(t *testing.T, root string) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "xdg"))
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
	t.Setenv("GIT_AUTHOR_NAME", "engram-test")
	t.Setenv("GIT_AUTHOR_EMAIL", "engram-test@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "engram-test")
	t.Setenv("GIT_COMMITTER_EMAIL", "engram-test@example.com")
}

// gitT runs a git command for test setup, failing the test on error.
func gitT(t *testing.T, dir string, args ...string) {
	t.Helper()
	if dir != "" {
		args = append([]string{"-C", dir}, args...)
	}
	if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// TestInitTeam exercises the whole clone → scaffold → commit → push flow against a
// local bare repo, with no network. It is skipped where git is unavailable.
func TestInitTeam(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	root := t.TempDir()
	hermeticGitEnv(t, root)

	bare := filepath.Join(root, "remote.git")
	gitT(t, "", "init", "--bare", bare)

	if err := InitTeam("file://" + bare); err != nil {
		t.Fatalf("InitTeam: %v", err)
	}

	// The clone landed in the managed dir and was scaffolded.
	dir, err := Dir()
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{
		"MEMORY.md",
		filepath.Join("global", ".gitkeep"),
		filepath.Join("projects", ".gitkeep"),
	} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("expected scaffolded %s: %v", f, err)
		}
	}

	// The scaffold was pushed: a fresh clone of the bare repo sees MEMORY.md.
	verify := filepath.Join(root, "verify")
	gitT(t, "", "clone", "file://"+bare, verify)
	if _, err := os.Stat(filepath.Join(verify, "MEMORY.md")); err != nil {
		t.Errorf("scaffold was not pushed to the remote: %v", err)
	}

	// Re-running refuses the now-populated team dir, with the expected message.
	err = InitTeam("file://" + bare)
	if err == nil {
		t.Fatal("expected error re-initializing an existing team dir")
	}
	if !strings.Contains(err.Error(), "already initialized") {
		t.Errorf("re-init error = %q, want it to mention 'already initialized'", err)
	}
}

// TestInitTeamLeavesPopulatedRepo confirms cloning a remote that already has
// commits does NOT scaffold over it — protecting an existing team's memories.
func TestInitTeamLeavesPopulatedRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	root := t.TempDir()
	hermeticGitEnv(t, root)

	bare := filepath.Join(root, "remote.git")
	gitT(t, "", "init", "--bare", bare)

	// Seed the remote with existing content via a scratch clone.
	seed := filepath.Join(root, "seed")
	gitT(t, "", "clone", "file://"+bare, seed)
	if err := os.WriteFile(filepath.Join(seed, "MEMORY.md"), []byte("# existing team\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitT(t, seed, "add", "-A")
	gitT(t, seed, "commit", "-m", "seed existing memories")
	gitT(t, seed, "push", "-u", "origin", "HEAD")

	if err := InitTeam("file://" + bare); err != nil {
		t.Fatalf("InitTeam: %v", err)
	}

	dir, err := Dir()
	if err != nil {
		t.Fatal(err)
	}
	// Existing content is preserved untouched...
	data, err := os.ReadFile(filepath.Join(dir, "MEMORY.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "# existing team\n" {
		t.Errorf("populated repo MEMORY.md = %q, want it left untouched", data)
	}
	// ...and the scaffold did not run.
	if _, err := os.Stat(filepath.Join(dir, "global", ".gitkeep")); err == nil {
		t.Error("scaffold ran on a populated repo (global/.gitkeep was created)")
	}
}
