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

	pushed, err := Withdraw(memPath)
	if err != nil {
		t.Fatalf("Withdraw: %v", err)
	}
	if !pushed {
		t.Error("expected the withdrawal to push to the bare remote")
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

	// The id is now tombstoned in the withdrawn ledger.
	if !readWithdrawn(teamDir)[meta.ID] {
		t.Error("withdrawn id should be recorded in the ledger")
	}

	// Withdrawing a memory that isn't team-scoped is refused (it's now personal).
	if _, err := Withdraw(memPath); err == nil {
		t.Error("expected an error withdrawing a memory that isn't team-scoped")
	}
}

func TestLedger(t *testing.T) {
	dir := t.TempDir()
	if readWithdrawn(dir)["A"] {
		t.Error("empty ledger should have nothing")
	}
	if addWithdrawn(dir, "A", "a.md") == "" || !readWithdrawn(dir)["A"] {
		t.Fatal("add should record the id")
	}
	if addWithdrawn(dir, "A", "a.md") != "" {
		t.Error("re-adding an existing id should be a no-op")
	}
	addWithdrawn(dir, "B", "b.md")
	if removeWithdrawn(dir, "A") == "" || readWithdrawn(dir)["A"] {
		t.Error("remove should drop A")
	}
	if !readWithdrawn(dir)["B"] {
		t.Error("remove A must not drop B")
	}
}

func TestWithdrawOwnerGuard(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	root := t.TempDir()
	hermeticGitEnv(t, root)
	cfg := filepath.Join(root, "gitconfig")
	setEmail := func(e string) {
		if err := os.WriteFile(cfg, []byte("[user]\n\tname = U\n\temail = "+e+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	setEmail("alice@example.com")
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
	if _, err := Promote(memPath, "global"); err != nil { // owner = alice
		t.Fatalf("Promote: %v", err)
	}

	setEmail("bob@example.com") // a different user
	if _, err := Withdraw(memPath); err == nil {
		t.Error("a non-owner must be blocked from withdrawing")
	}
	setEmail("alice@example.com") // the owner
	if _, err := Withdraw(memPath); err != nil {
		t.Errorf("the owner should be allowed to withdraw: %v", err)
	}
}

func TestPullPropagatesWithdrawal(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	root := t.TempDir()
	hermeticGitEnv(t, root)
	cfg := filepath.Join(root, "gitconfig")
	if err := os.WriteFile(cfg, []byte("[user]\n\tname = P\n\temail = p@example.com\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", cfg)
	bare := filepath.Join(root, "remote.git")
	gitT(t, "", "init", "--bare", bare)
	if err := InitTeam("file://" + bare); err != nil {
		t.Fatalf("InitTeam: %v", err)
	}
	teamDir, _ := Dir()

	// A teammate's local project: a pulled team memory (WD-1), a personal memory
	// that happens to share a withdrawn id (WD-2), and a team memory that was
	// re-promoted so its copy is back in the store (WD-3).
	memDir := filepath.Join(root, "proj", "memory")
	key := "github.com/acme/app"
	write := func(name, body string) {
		if err := os.MkdirAll(memDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(memDir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mem := func(id, scope string) string {
		return "---\nname: m\nengram:\n    id: " + id + "\n    scope: " + scope + "\n    project: " + key + "\n---\n# M\n\nbody\n"
	}
	write("shared.md", mem("WD-1", "team"))
	write("mine.md", mem("WD-2", "personal"))
	write("kept.md", mem("WD-3", "team"))
	write("global.md", mem("WD-4", "team")) // pulled from this project, but also shared globally

	// All four ids are tombstoned, but WD-3 is still in the store under this project
	// (re-promoted) and WD-4 is still in the store under global/ (a cross-scope copy).
	addWithdrawn(teamDir, "WD-1", "shared.md")
	addWithdrawn(teamDir, "WD-2", "mine.md")
	addWithdrawn(teamDir, "WD-3", "kept.md")
	addWithdrawn(teamDir, "WD-4", "global.md")
	writeStoreFile := func(rel, body string) {
		p := filepath.Join(teamDir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeStoreFile("projects/"+key+"/kept.md", mem("WD-3", "team"))
	writeStoreFile("global/global.md", mem("WD-4", "team"))

	res, err := Pull([]ProjectTarget{{Key: key, MemoryDir: memDir}})
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if res.Removed != 1 {
		t.Errorf("Removed = %d, want 1 (only the withdrawn team copy)", res.Removed)
	}
	if _, err := os.Stat(filepath.Join(memDir, "shared.md")); !os.IsNotExist(err) {
		t.Error("withdrawn team memory should be removed")
	}
	if _, err := os.Stat(filepath.Join(memDir, "mine.md")); err != nil {
		t.Error("a personal memory must never be removed")
	}
	if _, err := os.Stat(filepath.Join(memDir, "kept.md")); err != nil {
		t.Error("a re-promoted memory (still in the store) must not be removed")
	}
	if _, err := os.Stat(filepath.Join(memDir, "global.md")); err != nil {
		t.Error("a memory still shared in the store under another scope must not be removed")
	}
}
