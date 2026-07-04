package team

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/ertugrulhaskan/engram/internal/memory"
)

func TestWithdraw(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	root := t.TempDir()
	hermeticGitEnv(t, root)
	cfg := filepath.Join(root, "gitconfig")
	if err := os.WriteFile(cfg, []byte("[user]\n\tname = W\n\temail = w@example.com\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", cfg)

	bare := filepath.Join(root, "remote.git")
	gitT(t, "", "init", "--bare", bare)
	if err := InitTeam("file://" + bare); err != nil {
		t.Fatalf("InitTeam: %v", err)
	}

	memDir := filepath.Join(root, "proj", "memory")
	if err := os.MkdirAll(memDir, 0o755); err != nil {
		t.Fatal(err)
	}
	memPath := filepath.Join(memDir, "note.md")
	if err := os.WriteFile(memPath, []byte("---\nname: note\n---\n# Note\n\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Promote first, so there's a shared copy to withdraw.
	if _, err := Promote(memPath, "github.com/acme/app"); err != nil {
		t.Fatalf("Promote: %v", err)
	}
	teamDir, _ := Dir()
	storeCopy := filepath.Join(teamDir, "projects", "github.com/acme/app", "note.md")
	if _, err := os.Stat(storeCopy); err != nil {
		t.Fatalf("precondition: store copy should exist: %v", err)
	}

	removed, pushed, err := Withdraw(memPath)
	if err != nil {
		t.Fatalf("Withdraw: %v", err)
	}
	if !removed || !pushed {
		t.Errorf("expected the store copy removed and pushed, got removed=%v pushed=%v", removed, pushed)
	}

	// The store copy is gone locally...
	if _, err := os.Stat(storeCopy); !os.IsNotExist(err) {
		t.Errorf("store copy should be removed, stat err=%v", err)
	}
	// ...and on the remote.
	verify := filepath.Join(root, "verify")
	gitT(t, "", "clone", "file://"+bare, verify)
	if _, err := os.Stat(filepath.Join(verify, "projects", "github.com/acme/app", "note.md")); !os.IsNotExist(err) {
		t.Errorf("withdrawal not pushed; remote still has the copy (err=%v)", err)
	}

	// The local file is now personal, id preserved, Claude's key intact.
	raw, _ := os.ReadFile(memPath)
	meta, ok, _ := memory.ReadEngram(string(raw))
	if !ok || meta.Scope != "personal" || meta.ID == "" {
		t.Errorf("local engram after withdraw = %+v (ok=%v)", meta, ok)
	}

	// Withdrawing a memory that isn't team-scoped is refused (it's now personal).
	if _, _, err := Withdraw(memPath); err == nil {
		t.Error("expected an error withdrawing a memory that isn't team-scoped")
	}
}
