package team

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSameContent(t *testing.T) {
	if !sameContent("a\r\nb\n", "a\nb\n") {
		t.Error("CRLF vs LF content should compare equal")
	}
	if sameContent("alpha\n", "beta\n") {
		t.Error("different content should not compare equal")
	}
}

func TestPull(t *testing.T) {
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

	// A teammate clones the same store, adds a project-scoped memory, and pushes.
	mate := filepath.Join(root, "mate")
	gitT(t, "", "clone", "file://"+bare, mate)
	shared := "---\nname: shared\nengram:\n    id: ID-1\n    scope: team\n    project: github.com/acme/app\n---\n# Shared\n\nteam note\n"
	matePath := filepath.Join(mate, "projects", "github.com/acme/app", "shared.md")
	if err := os.MkdirAll(filepath.Dir(matePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(matePath, []byte(shared), 0o644); err != nil {
		t.Fatal(err)
	}
	gitT(t, mate, "add", "-A")
	gitT(t, mate, "commit", "-m", "add shared")
	gitT(t, mate, "push")

	// A local project whose remote normalizes to github.com/acme/app.
	localMem := filepath.Join(root, "myproj", "memory")
	if err := os.MkdirAll(localMem, 0o755); err != nil {
		t.Fatal(err)
	}
	targets := []ProjectTarget{{Key: "github.com/acme/app", MemoryDir: localMem}}

	// First pull: the team memory lands locally and is indexed.
	res, err := Pull(targets)
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if res.Placed != 1 {
		t.Errorf("Placed = %d, want 1 (%+v)", res.Placed, res)
	}
	if _, err := os.Stat(filepath.Join(localMem, "shared.md")); err != nil {
		t.Errorf("shared.md not placed: %v", err)
	}
	if idx, _ := os.ReadFile(filepath.Join(localMem, "MEMORY.md")); !strings.Contains(string(idx), "shared.md") {
		t.Errorf("MEMORY.md not reconciled:\n%s", idx)
	}

	// Re-pull: identical content → up to date, nothing placed or conflicting.
	if res2, err := Pull(targets); err != nil || res2.UpToDate != 1 || res2.Placed != 0 || res2.Conflicts != 0 {
		t.Errorf("re-pull = %+v err=%v, want UpToDate=1", res2, err)
	}

	// Conflict: a local edit makes the file differ → conflict, NOT overwritten.
	if err := os.WriteFile(filepath.Join(localMem, "shared.md"), []byte(shared+"\nlocal edit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res3, err := Pull(targets)
	if err != nil {
		t.Fatalf("conflict Pull: %v", err)
	}
	if res3.Conflicts != 1 {
		t.Errorf("conflict pull = %+v, want Conflicts=1", res3)
	}
	if got, _ := os.ReadFile(filepath.Join(localMem, "shared.md")); !strings.Contains(string(got), "local edit") {
		t.Error("conflict pull overwrote the local edit")
	}

	// Skip: a team memory whose project key has no local target.
	res4, err := Pull([]ProjectTarget{{Key: "github.com/other/repo", MemoryDir: localMem}})
	if err != nil {
		t.Fatalf("skip Pull: %v", err)
	}
	if res4.Skipped != 1 {
		t.Errorf("skip pull = %+v, want Skipped=1", res4)
	}
}
