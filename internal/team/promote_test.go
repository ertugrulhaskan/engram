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

// batchStore stands up a hermetic store with a bare remote and returns the store
// root plus a writeMem helper, so the batch tests below share one setup.
func batchStore(t *testing.T) (root, bare string, writeMem func(name, body string) string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	root = t.TempDir()
	hermeticGitEnv(t, root)
	cfg := filepath.Join(root, "gitconfig")
	if err := os.WriteFile(cfg, []byte("[user]\n\tname = Promoter\n\temail = promoter@example.com\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", cfg)

	bare = filepath.Join(root, "remote.git")
	gitT(t, "", "init", "--bare", bare)
	if err := InitTeam("file://" + bare); err != nil {
		t.Fatalf("InitTeam: %v", err)
	}

	memDir := filepath.Join(root, "proj", "memory")
	if err := os.MkdirAll(memDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeMem = func(name, body string) string {
		p := filepath.Join(memDir, name)
		if err := os.WriteFile(p, []byte("---\nname: "+strings.TrimSuffix(name, ".md")+
			"\nmetadata:\n  type: project\n---\n"+body+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	return root, bare, writeMem
}

// A batch is one unit of history: three memories, spanning two different
// placements, produce exactly ONE commit and arrive at the remote together.
// Promoting them one at a time is what this replaces — that would be three
// commits and three pushes for a single act.
func TestPromoteBatchIsOneCommit(t *testing.T) {
	root, bare, writeMem := batchStore(t)
	teamDir, _ := Dir()
	before := headSHA(t, teamDir)

	key := "github.com/acme/app"
	items := []PromoteItem{
		{Path: writeMem("one.md", "first"), Placement: key},
		{Path: writeMem("two.md", "second"), Placement: key},
		{Path: writeMem("three.md", "third"), Placement: "global"},
	}
	pushed, err := PromoteBatch(items)
	if err != nil {
		t.Fatalf("PromoteBatch: %v", err)
	}
	if !pushed {
		t.Error("expected the push to the local bare remote to succeed")
	}

	// Exactly one commit — the whole point of the batch.
	out, err := exec.Command("git", "-C", teamDir, "rev-list", "--count", before+"..HEAD").CombinedOutput()
	if err != nil {
		t.Fatalf("rev-list: %v\n%s", err, out)
	}
	if got := strings.TrimSpace(string(out)); got != "1" {
		t.Errorf("batch of 3 produced %s commits, want 1", got)
	}

	// Each item honoured its own placement, and all three reached the remote.
	verify := filepath.Join(root, "verify")
	gitT(t, "", "clone", "file://"+bare, verify)
	for _, rel := range []string{
		filepath.Join("projects", key, "one.md"),
		filepath.Join("projects", key, "two.md"),
		filepath.Join("global", "three.md"),
	} {
		if _, err := os.Stat(filepath.Join(verify, rel)); err != nil {
			t.Errorf("not pushed: %s (%v)", rel, err)
		}
	}

	// Every local copy is stamped with the placement it was promoted under.
	for _, it := range items {
		raw, _ := os.ReadFile(it.Path)
		meta, ok, err := memory.ReadEngram(string(raw))
		if err != nil || !ok {
			t.Fatalf("%s not stamped: ok=%v err=%v", filepath.Base(it.Path), ok, err)
		}
		if meta.Scope != "team" || meta.Project != it.Placement || meta.ID == "" {
			t.Errorf("%s engram = %+v, want scope=team project=%s", filepath.Base(it.Path), meta, it.Placement)
		}
	}
}

// Two memories in one batch that would land on the same store path must be
// refused — and refused before anything is written, so no local file is stamped
// and the store keeps a clean tree. On disk neither copy exists yet, so the
// existing on-disk collision check cannot see this one.
func TestPromoteBatchRefusesInternalCollision(t *testing.T) {
	root, _, writeMem := batchStore(t)
	teamDir, _ := Dir()
	before := headSHA(t, teamDir)

	// Same basename, two different source directories, both headed to global/.
	other := filepath.Join(root, "proj2", "memory")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	dup := filepath.Join(other, "notes.md")
	if err := os.WriteFile(dup, []byte("---\nname: notes\n---\nelsewhere\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	first := writeMem("notes.md", "here")

	_, err := PromoteBatch([]PromoteItem{
		{Path: first, Placement: "global"},
		{Path: dup, Placement: "global"},
	})
	if err == nil {
		t.Fatal("expected a refusal when two items claim the same store path")
	}
	if !strings.Contains(err.Error(), "notes.md") {
		t.Errorf("error should name the colliding file, got %q", err)
	}

	// Nothing was written: no commit, and neither local file was stamped.
	if after := headSHA(t, teamDir); after != before {
		t.Errorf("a refused batch committed:\n before %s\n after  %s", before, after)
	}
	for _, p := range []string{first, dup} {
		raw, _ := os.ReadFile(p)
		if _, ok, _ := memory.ReadEngram(string(raw)); ok {
			t.Errorf("%s was stamped despite the batch being refused", p)
		}
	}
}

// A batch that fails preparation for any reason leaves every local file alone —
// checked here with an unsafe placement in the second slot, so the first item
// would already have been written under a per-item apply loop.
func TestPromoteBatchRejectsBeforeWriting(t *testing.T) {
	_, _, writeMem := batchStore(t)
	teamDir, _ := Dir()
	before := headSHA(t, teamDir)

	good := writeMem("good.md", "fine")
	bad := writeMem("bad.md", "also fine, bad key")

	if _, err := PromoteBatch([]PromoteItem{
		{Path: good, Placement: "github.com/acme/app"},
		{Path: bad, Placement: "../../etc"},
	}); err == nil {
		t.Fatal("expected an unsafe project key to be rejected")
	}
	if after := headSHA(t, teamDir); after != before {
		t.Error("a rejected batch committed")
	}
	raw, _ := os.ReadFile(good)
	if _, ok, _ := memory.ReadEngram(string(raw)); ok {
		t.Error("the first item was stamped even though a later item was rejected")
	}
}

// An empty batch is a caller mistake, not a silent no-op: it must not reach git.
func TestPromoteBatchEmpty(t *testing.T) {
	if _, err := PromoteBatch(nil); err == nil {
		t.Error("PromoteBatch(nil) should report an error")
	}
}

// The commit subject stays the historical "Promote <file>" for a single memory
// and counts for a batch, rather than running a list of names past a readable
// subject line.
func TestPromoteMessage(t *testing.T) {
	one := []preparedPromotion{{memPath: filepath.Join("x", "note.md")}}
	if got := promoteMessage(one); got != "Promote note.md" {
		t.Errorf("single = %q, want %q", got, "Promote note.md")
	}
	three := []preparedPromotion{{memPath: "a.md"}, {memPath: "b.md"}, {memPath: "c.md"}}
	if got := promoteMessage(three); got != "Promote 3 memories" {
		t.Errorf("batch = %q, want %q", got, "Promote 3 memories")
	}
}

// On a case-insensitive filesystem (macOS, Windows) "Notes.md" and "notes.md" are
// the SAME file, so a batch carrying both would silently lose one — neither copy
// exists at prepare time, so the on-disk collision check that protects the
// single-memory path cannot see it. The batch check compares case-folded.
func TestPromoteBatchRefusesCaseOnlyCollision(t *testing.T) {
	root, _, writeMem := batchStore(t)
	teamDir, _ := Dir()
	before := headSHA(t, teamDir)

	other := filepath.Join(root, "p2")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	upper := filepath.Join(other, "Notes.md")
	if err := os.WriteFile(upper, []byte("---\nname: Notes\n---\nUPPER\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	lower := writeMem("notes.md", "LOWER")

	_, err := PromoteBatch([]PromoteItem{
		{Path: upper, Placement: "global"},
		{Path: lower, Placement: "global"},
	})
	if err == nil {
		t.Fatal("a case-only filename collision must be refused, not silently merged")
	}
	if after := headSHA(t, teamDir); after != before {
		t.Error("the refused batch still committed")
	}
	for _, p := range []string{upper, lower} {
		raw, _ := os.ReadFile(p)
		if _, ok, _ := memory.ReadEngram(string(raw)); ok {
			t.Errorf("%s was stamped despite the refusal", filepath.Base(p))
		}
	}
}
