package team

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ertugrulhaskan/engram/internal/memory"
)

func TestConflictResolveFrontmatterAndMarkers(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	root := t.TempDir()
	hermeticGitEnv(t, root)
	cfg := filepath.Join(root, "gitconfig")
	if err := os.WriteFile(cfg, []byte("[user]\n\tname = R\n\temail = r@example.com\n"), 0o644); err != nil {
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
	if _, err := Promote(memPath, "global"); err != nil {
		t.Fatalf("Promote: %v", err)
	}

	// Diverge ONLY in Claude frontmatter (description) — bodies stay identical.
	teamDir, _ := Dir()
	storePath := filepath.Join(teamDir, "global", "note.md")
	sraw, _ := os.ReadFile(storePath)
	sm, _, _ := memory.ReadEngram(string(sraw))
	teamContent := "---\nname: note\ndescription: team desc\n---\n# Note\n\nbody\n"
	sm.SyncedHash, _ = memory.ContentDigest(teamContent)
	newStore, _ := memory.WriteEngram(teamContent, sm)
	if err := os.WriteFile(storePath, []byte(newStore), 0o644); err != nil {
		t.Fatal(err)
	}
	lraw, _ := os.ReadFile(memPath)
	lm, _, _ := memory.ReadEngram(string(lraw)) // keeps the base SyncedHash
	localFull, _ := memory.WriteEngram("---\nname: note\ndescription: my desc\n---\n# Note\n\nbody\n", lm)
	if err := os.WriteFile(memPath, []byte(localFull), 0o644); err != nil {
		t.Fatal(err)
	}

	// The frontmatter divergence is visible in the conflict editor (both descriptions).
	tmp, err := BeginConflictResolve(memPath)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	txt, _ := os.ReadFile(tmp)
	if !strings.Contains(string(txt), "my desc") || !strings.Contains(string(txt), "team desc") {
		t.Errorf("frontmatter divergence not surfaced:\n%s", txt)
	}

	// A lone "=======" divider (outer markers deleted) is still refused.
	if err := os.WriteFile(tmp, []byte("# Note\n\nx\n=======\n# Note\n\ny\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if resolved, err := FinishConflictResolve(memPath, tmp); resolved || err == nil {
		t.Errorf("a lone ======= divider must be refused: resolved=%v err=%v", resolved, err)
	}

	// Take theirs → the local description becomes the team's, and it reads synced.
	tmp2, err := BeginConflictResolve(memPath)
	if err != nil {
		t.Fatalf("Begin (2): %v", err)
	}
	takeTheirs, _ := memory.ShareContent(newStore)
	if err := os.WriteFile(tmp2, []byte(takeTheirs), 0o644); err != nil {
		t.Fatal(err)
	}
	if resolved, err := FinishConflictResolve(memPath, tmp2); err != nil || !resolved {
		t.Fatalf("resolve: resolved=%v err=%v", resolved, err)
	}
	got, _ := os.ReadFile(memPath)
	if !strings.Contains(string(got), "team desc") || strings.Contains(string(got), "my desc") {
		t.Errorf("frontmatter not reconciled to theirs:\n%s", got)
	}
	if st, _ := SyncStates([]memory.Memory{{Path: memPath, Raw: string(got)}}); st[memPath] != StateSynced {
		t.Errorf("after taking theirs, state = %v, want StateSynced", st[memPath])
	}
}

func TestConflictResolve(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	root := t.TempDir()
	hermeticGitEnv(t, root)
	cfg := filepath.Join(root, "gitconfig")
	if err := os.WriteFile(cfg, []byte("[user]\n\tname = R\n\temail = r@example.com\n"), 0o644); err != nil {
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
	if err := os.WriteFile(memPath, []byte("---\nname: note\n---\n# Note\n\nlocal body\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Promote(memPath, "global"); err != nil {
		t.Fatalf("Promote: %v", err)
	}

	// Diverge: the store copy advances (restamped), and the local body is edited.
	teamDir, _ := Dir()
	storePath := filepath.Join(teamDir, "global", "note.md")
	sraw, _ := os.ReadFile(storePath)
	sm, _, _ := memory.ReadEngram(string(sraw))
	newStore := memory.SetBody(string(sraw), "# Note\n\nteam body\n")
	sm.SyncedHash, _ = memory.ContentDigest(newStore)
	newStore, _ = memory.WriteEngram(newStore, sm)
	if err := os.WriteFile(storePath, []byte(newStore), 0o644); err != nil {
		t.Fatal(err)
	}
	lraw, _ := os.ReadFile(memPath)
	if err := os.WriteFile(memPath, []byte(memory.SetBody(string(lraw), "# Note\n\nmy body\n")), 0o644); err != nil {
		t.Fatal(err)
	}

	// Begin: a temp merge holding both bodies with markers.
	tmp, err := BeginConflictResolve(memPath)
	if err != nil {
		t.Fatalf("BeginConflictResolve: %v", err)
	}
	txt, _ := os.ReadFile(tmp)
	if !strings.Contains(string(txt), "my body") || !strings.Contains(string(txt), "team body") {
		t.Errorf("conflict text missing a side:\n%s", txt)
	}

	// Saving with markers still present is refused, and the memory is untouched.
	if resolved, err := FinishConflictResolve(memPath, tmp); resolved || err == nil {
		t.Errorf("unresolved markers should be refused: resolved=%v err=%v", resolved, err)
	}
	if got, _ := os.ReadFile(memPath); !strings.Contains(string(got), "my body") {
		t.Errorf("a refused resolve must not change the memory:\n%s", got)
	}

	// Resolve by taking the team body; the memory updates and reads as synced.
	tmp2, err := BeginConflictResolve(memPath)
	if err != nil {
		t.Fatalf("BeginConflictResolve (2): %v", err)
	}
	takeTheirs, err := memory.ShareContent(newStore) // resolve by adopting the team's shared content
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tmp2, []byte(takeTheirs), 0o644); err != nil {
		t.Fatal(err)
	}
	resolved, err := FinishConflictResolve(memPath, tmp2)
	if err != nil || !resolved {
		t.Fatalf("resolve: resolved=%v err=%v", resolved, err)
	}
	if _, err := os.Stat(tmp2); !os.IsNotExist(err) {
		t.Errorf("resolve temp file should be cleaned up, stat err=%v", err)
	}
	got, _ := os.ReadFile(memPath)
	if !strings.Contains(string(got), "team body") || strings.Contains(string(got), "my body") {
		t.Errorf("local not updated to the resolution:\n%s", got)
	}
	gm, ok, _ := memory.ReadEngram(string(got))
	if !ok || gm.Scope != "team" || gm.ID == "" {
		t.Errorf("engram block lost after resolve: %+v ok=%v", gm, ok)
	}
	st, _ := SyncStates([]memory.Memory{{Path: memPath, Raw: string(got)}})
	if st[memPath] != StateSynced {
		t.Errorf("after taking theirs, state = %v, want StateSynced", st[memPath])
	}
}
