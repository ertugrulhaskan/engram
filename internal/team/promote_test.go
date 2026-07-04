package team

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ertugrulhaskan/engram/internal/memory"
)

func TestPlacementPath(t *testing.T) {
	if p, err := placementPath("global", "x.md"); err != nil || p != filepath.Join("global", "x.md") {
		t.Errorf("global: p=%q err=%v", p, err)
	}
	if p, err := placementPath("github.com/acme/app", "x.md"); err != nil ||
		p != filepath.Join("projects", "github.com/acme/app", "x.md") {
		t.Errorf("project: p=%q err=%v", p, err)
	}
	for _, bad := range []string{"..", "../etc", "github.com/../../etc", "/abs", ""} {
		if _, err := placementPath(bad, "x.md"); err == nil {
			t.Errorf("placementPath(%q) should be rejected", bad)
		}
	}
}

func TestPromote(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	root := t.TempDir()
	hermeticGitEnv(t, root)
	// A global git config that carries an email, so owner is populated and commits
	// have an identity (overrides hermeticGitEnv's /dev/null global).
	cfg := filepath.Join(root, "gitconfig")
	if err := os.WriteFile(cfg, []byte("[user]\n\tname = Promoter\n\temail = promoter@example.com\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", cfg)

	bare := filepath.Join(root, "remote.git")
	gitT(t, "", "init", "--bare", bare)
	if err := InitTeam("file://" + bare); err != nil {
		t.Fatalf("InitTeam: %v", err)
	}

	// A local personal memory with Claude frontmatter.
	memDir := filepath.Join(root, "proj", "memory")
	if err := os.MkdirAll(memDir, 0o755); err != nil {
		t.Fatal(err)
	}
	memPath := filepath.Join(memDir, "my-note.md")
	if err := os.WriteFile(memPath, []byte("---\nname: my-note\nmetadata:\n  type: project\n---\n# My note\n\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	pushed, err := Promote(memPath, "github.com/acme/app")
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if !pushed {
		t.Error("expected push to the local bare remote to succeed")
	}

	// The local file is stamped: team scope, an id, the project, and owner — with
	// Claude's key intact.
	raw, _ := os.ReadFile(memPath)
	meta, ok, err := memory.ReadEngram(string(raw))
	if err != nil || !ok {
		t.Fatalf("local not stamped: ok=%v err=%v", ok, err)
	}
	if meta.Scope != "team" || meta.Project != "github.com/acme/app" || meta.ID == "" {
		t.Errorf("local engram = %+v", meta)
	}
	if meta.Owner != "promoter@example.com" {
		t.Errorf("owner = %q, want promoter@example.com", meta.Owner)
	}
	// The sync anchor is stamped and equals the digest of the shared content
	// (ContentDigest strips the engram block, so it recomputes the same value).
	if want, _ := memory.ContentDigest(string(raw)); meta.SyncedHash == "" || meta.SyncedHash != want {
		t.Errorf("syncedHash = %q, want %q", meta.SyncedHash, want)
	}
	if !strings.Contains(string(raw), "name: my-note") {
		t.Errorf("lost Claude key locally:\n%s", raw)
	}

	// The team copy is placed and pushed to the remote.
	teamDir, _ := Dir()
	if _, err := os.Stat(filepath.Join(teamDir, "projects", "github.com/acme/app", "my-note.md")); err != nil {
		t.Errorf("team copy missing: %v", err)
	}
	verify := filepath.Join(root, "verify")
	gitT(t, "", "clone", "file://"+bare, verify)
	if _, err := os.Stat(filepath.Join(verify, "projects", "github.com/acme/app", "my-note.md")); err != nil {
		t.Errorf("promote was not pushed: %v", err)
	}

	// Re-promoting unchanged content is a no-op (no new commit).
	before := headSHA(t, teamDir)
	if _, err := Promote(memPath, "github.com/acme/app"); err != nil {
		t.Fatalf("re-Promote: %v", err)
	}
	if after := headSHA(t, teamDir); after != before {
		t.Errorf("re-promote created a commit:\n before %s\n after  %s", before, after)
	}
}

func TestPromoteRefusesGlobalCollision(t *testing.T) {
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

	write := func(p, body string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	a := filepath.Join(root, "projA", "memory", "note.md")
	b := filepath.Join(root, "projB", "memory", "note.md")
	write(a, "# A\n\nalpha\n")
	write(b, "# B\n\nbeta\n")

	if _, err := Promote(a, "global"); err != nil {
		t.Fatalf("promote A: %v", err)
	}
	// A different memory with the same basename promoted to the same global path
	// must be refused, not silently overwrite A.
	if _, err := Promote(b, "global"); err == nil {
		t.Error("expected a collision error promoting a different note.md to global")
	}
	// The refused promote happened before any write, so B's local file is untouched.
	raw, _ := os.ReadFile(b)
	if _, ok, _ := memory.ReadEngram(string(raw)); ok {
		t.Error("a refused promote must not stamp the local file")
	}
}

func headSHA(t *testing.T, dir string) string {
	t.Helper()
	out, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD").CombinedOutput()
	if err != nil {
		t.Fatalf("rev-parse: %v\n%s", err, out)
	}
	return strings.TrimSpace(string(out))
}
