package team

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ertugrulhaskan/engram/internal/memory"
)

func TestSameContent(t *testing.T) {
	if !sameContent("a\r\nb\n", "a\nb\n") {
		t.Error("CRLF vs LF content should compare equal")
	}
	if sameContent("alpha\n", "beta\n") {
		t.Error("different content should not compare equal")
	}
}

func TestPullFastForwardVsDiverge(t *testing.T) {
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

	key := "github.com/acme/app"
	localMem := filepath.Join(root, "myproj", "memory")
	if err := os.MkdirAll(localMem, 0o755); err != nil {
		t.Fatal(err)
	}
	writeMem := func(name, body string) string {
		p := filepath.Join(localMem, name)
		raw := "---\nname: " + strings.TrimSuffix(name, ".md") + "\n---\n# " + name + "\n\n" + body + "\n"
		if err := os.WriteFile(p, []byte(raw), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	// Promote two memories: both anchored, both copied into the store at the base.
	if _, err := Promote(writeMem("ff.md", "v1"), key); err != nil {
		t.Fatalf("Promote ff: %v", err)
	}
	if _, err := Promote(writeMem("div.md", "v1"), key); err != nil {
		t.Fatalf("Promote div: %v", err)
	}
	if _, err := Promote(writeMem("ahead.md", "v1"), key); err != nil {
		t.Fatalf("Promote ahead: %v", err)
	}
	targets := []ProjectTarget{{Key: key, MemoryDir: localMem}}

	// A teammate clones and advances BOTH store copies (restamping the anchor), then pushes.
	mate := filepath.Join(root, "mate")
	gitT(t, "", "clone", "file://"+bare, mate)
	advance := func(name, body string) {
		p := filepath.Join(mate, "projects", key, name)
		raw, _ := os.ReadFile(p)
		m, _, _ := memory.ReadEngram(string(raw))
		full := "---\nname: " + strings.TrimSuffix(name, ".md") + "\n---\n# " + name + "\n\n" + body + "\n"
		m.SyncedHash, _ = memory.ContentDigest(full)
		out, err := memory.WriteEngram(full, m)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(out), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	advance("ff.md", "v2 from mate")
	advance("div.md", "v2 from mate") // ahead.md is deliberately NOT advanced in the store
	gitT(t, mate, "add", "-A")
	gitT(t, mate, "commit", "-m", "advance both")
	gitT(t, mate, "push")

	// Locally edit div.md and ahead.md so they move off the base (ff.md stays untouched).
	for _, name := range []string{"div.md", "ahead.md"} {
		raw, _ := os.ReadFile(filepath.Join(localMem, name))
		if err := os.WriteFile(filepath.Join(localMem, name),
			[]byte(strings.Replace(string(raw), "v1", "my local edit", 1)), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	res, err := Pull(targets)
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	// ff.md: local untouched, store advanced → fast-forward. div.md: both moved →
	// conflict. ahead.md: only local moved, store at base → local-ahead (counted, left).
	if res.Updated != 1 || res.Conflicts != 1 || res.Ahead != 1 {
		t.Errorf("pull = %+v, want Updated=1 Conflicts=1 Ahead=1", res)
	}
	if got, _ := os.ReadFile(filepath.Join(localMem, "ff.md")); !strings.Contains(string(got), "v2 from mate") {
		t.Errorf("ff.md not fast-forwarded:\n%s", got)
	}
	if got, _ := os.ReadFile(filepath.Join(localMem, "div.md")); !strings.Contains(string(got), "my local edit") {
		t.Errorf("div.md conflict was overwritten:\n%s", got)
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

// snapshotDir maps every file under dir to its content, so a test can prove a
// plan wrote nothing.
func snapshotDir(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(dir, path)
		raw, _ := os.ReadFile(path)
		out[rel] = string(raw)
		return nil
	})
	return out
}

// PullPlan must produce exactly the accounting the write path produces, while
// writing nothing; PullApply then applies a confirmed plan without re-fetching.
func TestPullPlanMatchesPull(t *testing.T) {
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

	localMem := filepath.Join(root, "myproj", "memory")
	if err := os.MkdirAll(localMem, 0o755); err != nil {
		t.Fatal(err)
	}
	targets := []ProjectTarget{{Key: "github.com/acme/app", MemoryDir: localMem}}

	// Stage 1 — fresh placement: the plan counts it, writes nothing; the apply
	// (no second fetch) lands it with the same count.
	before := snapshotDir(t, localMem)
	plan, err := PullPlan(targets)
	if err != nil {
		t.Fatalf("PullPlan: %v", err)
	}
	if plan.Placed != 1 {
		t.Errorf("plan = %+v, want Placed=1", plan)
	}
	if after := snapshotDir(t, localMem); len(after) != len(before) {
		t.Errorf("PullPlan wrote files: before=%v after=%v", before, after)
	}
	applied, err := PullApply(targets)
	if err != nil {
		t.Fatalf("PullApply: %v", err)
	}
	if applied != plan {
		t.Errorf("apply %+v disagrees with plan %+v", applied, plan)
	}
	if _, err := os.Stat(filepath.Join(localMem, "shared.md")); err != nil {
		t.Errorf("apply did not place shared.md: %v", err)
	}

	// Stage 2 — up to date: plan equals pull, still no writes.
	before = snapshotDir(t, localMem)
	plan2, err := PullPlan(targets)
	if err != nil {
		t.Fatalf("PullPlan 2: %v", err)
	}
	res2, err := Pull(targets)
	if err != nil {
		t.Fatalf("Pull 2: %v", err)
	}
	if plan2 != res2 || plan2.UpToDate != 1 {
		t.Errorf("plan %+v vs pull %+v, want equal with UpToDate=1", plan2, res2)
	}

	// Stage 3 — local edit: both classify it a conflict; the plan leaves the
	// edit untouched byte-for-byte.
	edited := shared + "\nlocal edit\n"
	if err := os.WriteFile(filepath.Join(localMem, "shared.md"), []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	plan3, err := PullPlan(targets)
	if err != nil {
		t.Fatalf("PullPlan 3: %v", err)
	}
	res3, err := Pull(targets)
	if err != nil {
		t.Fatalf("Pull 3: %v", err)
	}
	if plan3 != res3 || plan3.Conflicts != 1 {
		t.Errorf("plan %+v vs pull %+v, want equal with Conflicts=1", plan3, res3)
	}
	if got, _ := os.ReadFile(filepath.Join(localMem, "shared.md")); string(got) != edited {
		t.Error("a plan or conflict pull touched the locally edited file")
	}
}

// TestPullDemotesOwnWithdrawnCopy pins the demote path end to end: when the
// only pending work is resetting the owner's own withdrawn copy to
// scope:personal, the plan must count it (a demote rewrites the engram: block,
// so it is a write the user has to see) and the apply must actually perform it.
// Previously the plan counted nothing here, so the TUI's zero-work gate skipped
// PullApply entirely and the copy stayed scope:team forever.
func TestPullDemotesOwnWithdrawnCopy(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	root := t.TempDir()
	hermeticGitEnv(t, root)
	cfg := filepath.Join(root, "gitconfig")
	if err := os.WriteFile(cfg, []byte("[user]\n\tname = O\n\temail = owner@example.com\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", cfg)

	bare := filepath.Join(root, "remote.git")
	gitT(t, "", "init", "--bare", bare)
	if err := InitTeam("file://" + bare); err != nil {
		t.Fatalf("InitTeam: %v", err)
	}
	// A tombstone: the id is withdrawn upstream and present nowhere in the store.
	mate := filepath.Join(root, "mate")
	gitT(t, "", "clone", "file://"+bare, mate)
	tomb := filepath.Join(mate, withdrawnLedger)
	if err := os.WriteFile(tomb, []byte("ID-9 mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitT(t, mate, "add", "-A")
	gitT(t, mate, "commit", "-m", "withdraw ID-9")
	gitT(t, mate, "push")

	// The owner's other checkout still says scope:team, unedited since its sync.
	localMem := filepath.Join(root, "myproj", "memory")
	if err := os.MkdirAll(localMem, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\nname: mine\nengram:\n    id: ID-9\n    scope: team\n    owner: owner@example.com\n    project: github.com/acme/app\n---\n# Mine\n\nowned note\n"
	dig, err := memory.ContentDigest(body)
	if err != nil {
		t.Fatal(err)
	}
	withAnchor := strings.Replace(body, "    scope: team\n", "    scope: team\n    syncedHash: "+dig+"\n", 1)
	local := filepath.Join(localMem, "mine.md")
	if err := os.WriteFile(local, []byte(withAnchor), 0o644); err != nil {
		t.Fatal(err)
	}
	targets := []ProjectTarget{{Key: "github.com/acme/app", MemoryDir: localMem}}

	// The plan must disclose the demote and write nothing.
	plan, err := PullPlan(targets)
	if err != nil {
		t.Fatalf("PullPlan: %v", err)
	}
	if plan.Demoted != 1 {
		t.Fatalf("plan = %+v, want Demoted=1 (a demote-only pull must not look like no work)", plan)
	}
	if got, _ := os.ReadFile(local); string(got) != withAnchor {
		t.Error("PullPlan rewrote the file")
	}

	// The apply performs it, and agrees with the plan.
	applied, err := PullApply(targets)
	if err != nil {
		t.Fatalf("PullApply: %v", err)
	}
	if applied != plan {
		t.Errorf("apply %+v disagrees with plan %+v", applied, plan)
	}
	got, err := os.ReadFile(local)
	if err != nil {
		t.Fatal(err)
	}
	m, ok, _ := memory.ReadEngram(string(got))
	if !ok || m.Scope != "personal" {
		t.Errorf("scope = %q (ok=%v), want personal — the file must be kept and demoted", m.Scope, ok)
	}
	if !strings.Contains(string(got), "owned note") {
		t.Error("demote lost the memory's body")
	}
}

// TestPullReconcilesGlobalCopies pins the global half of pull. A global-scoped
// memory has no matching project, so pull must never *place* one — but a local
// copy the user already holds gets the same three-way reconcile as a project
// memory: fast-forward when only the store moved, leave a local-ahead copy,
// flag a genuine divergence, count an untouched pair up-to-date.
func TestPullReconcilesGlobalCopies(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	root := t.TempDir()
	hermeticGitEnv(t, root)
	cfg := filepath.Join(root, "gitconfig")
	if err := os.WriteFile(cfg, []byte("[user]\n\tname = G\n\temail = g@example.com\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", cfg)
	bare := filepath.Join(root, "remote.git")
	gitT(t, "", "init", "--bare", bare)
	if err := InitTeam("file://" + bare); err != nil {
		t.Fatalf("InitTeam: %v", err)
	}

	localMem := filepath.Join(root, "myproj", "memory")
	if err := os.MkdirAll(localMem, 0o755); err != nil {
		t.Fatal(err)
	}
	writeMem := func(name, body string) string {
		p := filepath.Join(localMem, name)
		raw := "---\nname: " + strings.TrimSuffix(name, ".md") + "\n---\n# " + name + "\n\n" + body + "\n"
		if err := os.WriteFile(p, []byte(raw), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	// Four global memories, one per reconcile outcome.
	for _, name := range []string{"gff.md", "gdiv.md", "gahead.md", "gsame.md"} {
		if _, err := Promote(writeMem(name, "v1"), "global"); err != nil {
			t.Fatalf("Promote %s: %v", name, err)
		}
	}
	targets := []ProjectTarget{{Key: "github.com/acme/app", MemoryDir: localMem}}

	// A teammate advances two of the store copies and shares a brand-new global
	// memory this machine has no copy of.
	mate := filepath.Join(root, "mate")
	gitT(t, "", "clone", "file://"+bare, mate)
	advance := func(name, body string) {
		p := filepath.Join(mate, "global", name)
		raw, _ := os.ReadFile(p)
		m, _, _ := memory.ReadEngram(string(raw))
		full := "---\nname: " + strings.TrimSuffix(name, ".md") + "\n---\n# " + name + "\n\n" + body + "\n"
		m.SyncedHash, _ = memory.ContentDigest(full)
		out, err := memory.WriteEngram(full, m)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(out), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	advance("gff.md", "v2 from mate")
	advance("gdiv.md", "v2 from mate") // gahead.md / gsame.md stay at base in the store
	newID, err := memory.NewID()
	if err != nil {
		t.Fatal(err)
	}
	newContent := "---\nname: gnew\n---\n# gnew.md\n\nnever seen here\n"
	newAnchor, _ := memory.ContentDigest(newContent)
	newRaw, err := memory.WriteEngram(newContent, memory.EngramMeta{
		ID: newID, Scope: "team", Project: "global", Owner: "mate@example.com", SyncedHash: newAnchor,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mate, "global", "gnew.md"), []byte(newRaw), 0o644); err != nil {
		t.Fatal(err)
	}
	gitT(t, mate, "add", "-A")
	gitT(t, mate, "commit", "-m", "advance globals, add gnew")
	gitT(t, mate, "push")

	// Locally edit gdiv.md and gahead.md off the base (gff.md and gsame.md stay).
	for _, name := range []string{"gdiv.md", "gahead.md"} {
		raw, _ := os.ReadFile(filepath.Join(localMem, name))
		if err := os.WriteFile(filepath.Join(localMem, name),
			[]byte(strings.Replace(string(raw), "v1", "my local edit", 1)), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	res, err := Pull(targets)
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	// gff: fast-forward. gdiv: conflict. gahead: local-ahead. gsame: up-to-date.
	// gnew: NOT placed and NOT counted — a global memory the user holds nowhere
	// stays in the store, it is not skipped placement.
	if want := (PullResult{Updated: 1, Conflicts: 1, Ahead: 1, UpToDate: 1}); res != want {
		t.Errorf("pull = %+v, want %+v", res, want)
	}
	if got, _ := os.ReadFile(filepath.Join(localMem, "gff.md")); !strings.Contains(string(got), "v2 from mate") {
		t.Errorf("gff.md not fast-forwarded:\n%s", got)
	}
	if got, _ := os.ReadFile(filepath.Join(localMem, "gdiv.md")); !strings.Contains(string(got), "my local edit") {
		t.Errorf("gdiv.md conflict was overwritten:\n%s", got)
	}
	if _, err := os.Stat(filepath.Join(localMem, "gnew.md")); !os.IsNotExist(err) {
		t.Errorf("gnew.md was placed locally (stat err = %v) — pull must never place a global memory", err)
	}
}

// TestPullGlobalIgnoresProjectScopedCopy pins the dual-scope guard: when the
// same id sits under both projects/<key>/ and global/ (a stale re-promote), the
// global walk must not reconcile a local copy that tracks the project scope —
// otherwise a diverged global copy would fast-forward over it. Mirrors the
// matching rule storeCopyRaw applies on resolve.
func TestPullGlobalIgnoresProjectScopedCopy(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	root := t.TempDir()
	hermeticGitEnv(t, root)
	cfg := filepath.Join(root, "gitconfig")
	if err := os.WriteFile(cfg, []byte("[user]\n\tname = G\n\temail = g@example.com\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", cfg)
	bare := filepath.Join(root, "remote.git")
	gitT(t, "", "init", "--bare", bare)
	if err := InitTeam("file://" + bare); err != nil {
		t.Fatalf("InitTeam: %v", err)
	}

	key := "github.com/acme/app"
	localMem := filepath.Join(root, "myproj", "memory")
	if err := os.MkdirAll(localMem, 0o755); err != nil {
		t.Fatal(err)
	}
	memPath := filepath.Join(localMem, "note.md")
	if err := os.WriteFile(memPath, []byte("---\nname: note\n---\n# Note\n\nproject truth\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Promote(memPath, key); err != nil {
		t.Fatalf("Promote: %v", err)
	}

	// A teammate commits a global/ copy of the SAME id whose content has advanced.
	mate := filepath.Join(root, "mate")
	gitT(t, "", "clone", "file://"+bare, mate)
	praw, _ := os.ReadFile(filepath.Join(mate, "projects", key, "note.md"))
	m, _, _ := memory.ReadEngram(string(praw))
	m.Project = "global"
	globalContent := "---\nname: note\n---\n# Note\n\nglobal drift\n"
	m.SyncedHash, _ = memory.ContentDigest(globalContent)
	graw, err := memory.WriteEngram(globalContent, m)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mate, "global", "note.md"), []byte(graw), 0o644); err != nil {
		t.Fatal(err)
	}
	gitT(t, mate, "add", "-A")
	gitT(t, mate, "commit", "-m", "stale dual-scope copy")
	gitT(t, mate, "push")

	res, err := Pull([]ProjectTarget{{Key: key, MemoryDir: localMem}})
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	// The projects walk counts the untouched pair up-to-date; the global walk must
	// contribute nothing — not a second count, and above all not a fast-forward.
	if want := (PullResult{UpToDate: 1}); res != want {
		t.Errorf("pull = %+v, want %+v", res, want)
	}
	if got, _ := os.ReadFile(memPath); !strings.Contains(string(got), "project truth") ||
		strings.Contains(string(got), "global drift") {
		t.Errorf("the global copy crossed scopes into the local file:\n%s", got)
	}
}

// A remoteless project can hold a global memory — promote falls back to global
// where there is no remote — so it can hold a *withdrawn* one. The tombstone loop
// used to skip every target with no key, which left that copy stamped scope:team
// forever: exactly the state the ledger exists to prevent. This is the companion
// to resolveTargets keeping remoteless projects as global-pull targets; without
// it, pull would update such a copy but never retire it.
func TestPullPropagatesWithdrawalToRemotelessProject(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	root := t.TempDir()
	hermeticGitEnv(t, root)
	cfg := filepath.Join(root, "gitconfig")
	if err := os.WriteFile(cfg, []byte("[user]\n\tname = O\n\temail = owner@example.com\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", cfg)

	bare := filepath.Join(root, "remote.git")
	gitT(t, "", "init", "--bare", bare)
	if err := InitTeam("file://" + bare); err != nil {
		t.Fatalf("InitTeam: %v", err)
	}
	mate := filepath.Join(root, "mate")
	gitT(t, "", "clone", "file://"+bare, mate)
	if err := os.WriteFile(filepath.Join(mate, withdrawnLedger), []byte("ID-9 mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitT(t, mate, "add", "-A")
	gitT(t, mate, "commit", "-m", "withdraw ID-9")
	gitT(t, mate, "push")

	// A global-scoped copy in a project with no git remote, unedited since sync.
	localMem := filepath.Join(root, "remoteless", "memory")
	if err := os.MkdirAll(localMem, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\nname: mine\nengram:\n    id: ID-9\n    scope: team\n    owner: owner@example.com\n---\n# Mine\n\nowned note\n"
	dig, err := memory.ContentDigest(body)
	if err != nil {
		t.Fatal(err)
	}
	withAnchor := strings.Replace(body, "    scope: team\n", "    scope: team\n    syncedHash: "+dig+"\n", 1)
	local := filepath.Join(localMem, "mine.md")
	if err := os.WriteFile(local, []byte(withAnchor), 0o644); err != nil {
		t.Fatal(err)
	}

	// The empty Key is the point: this project has no git remote.
	targets := []ProjectTarget{{Key: "", MemoryDir: localMem}}

	plan, err := PullPlan(targets)
	if err != nil {
		t.Fatalf("PullPlan: %v", err)
	}
	if plan.Demoted != 1 {
		t.Fatalf("plan = %+v, want Demoted=1 — the withdrawal never reached a remoteless project", plan)
	}

	applied, err := PullApply(targets)
	if err != nil {
		t.Fatalf("PullApply: %v", err)
	}
	if applied.Demoted != 1 {
		t.Fatalf("apply = %+v, want Demoted=1", applied)
	}
	got, err := os.ReadFile(local)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(got), "scope: team") {
		t.Error("the withdrawn copy is still stamped scope: team")
	}
	if !strings.Contains(string(got), "scope: personal") {
		t.Errorf("copy was not demoted to personal:\n%s", got)
	}
}

// The mirror of TestPullGlobalIgnoresProjectScopedCopy: here the local copy tracks
// global/ and the abandoned projects/<key>/ copy is the stale one. The projects walk
// used to reconcile with no scope guard at all, so it fast-forwarded the local file
// *backwards* onto the copy the user had already moved off — silent data loss reached
// by two ordinary actions (promote to a project, then promote the same memory globally).
func TestPullProjectWalkIgnoresGlobalScopedCopy(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	root := t.TempDir()
	hermeticGitEnv(t, root)
	cfg := filepath.Join(root, "gitconfig")
	if err := os.WriteFile(cfg, []byte("[user]\n\tname = G\n\temail = g@example.com\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", cfg)
	bare := filepath.Join(root, "remote.git")
	gitT(t, "", "init", "--bare", bare)
	if err := InitTeam("file://" + bare); err != nil {
		t.Fatalf("InitTeam: %v", err)
	}

	key := "github.com/acme/app"
	localMem := filepath.Join(root, "myproj", "memory")
	if err := os.MkdirAll(localMem, 0o755); err != nil {
		t.Fatal(err)
	}
	memPath := filepath.Join(localMem, "note.md")
	if err := os.WriteFile(memPath, []byte("---\nname: note\n---\n# Note\n\nproject truth\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Shared to the project first — this is the copy that goes stale.
	if _, err := Promote(memPath, key); err != nil {
		t.Fatalf("Promote to project: %v", err)
	}

	// The user edits it, then re-promotes the same memory globally. Promote restamps
	// engram.project to "global"; the projects/<key>/ copy is left behind in the store.
	raw, err := os.ReadFile(memPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(memPath, []byte(strings.Replace(string(raw), "project truth", "global truth", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Promote(memPath, "global"); err != nil {
		t.Fatalf("Promote to global: %v", err)
	}

	res, err := Pull([]ProjectTarget{{Key: key, MemoryDir: localMem}})
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	// The global walk sees an identical copy; the projects walk must contribute
	// nothing — above all not a fast-forward back onto the abandoned copy.
	if want := (PullResult{UpToDate: 1}); res != want {
		t.Errorf("Pull = %+v, want %+v", res, want)
	}
	after, err := os.ReadFile(memPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(after), "global truth") {
		t.Error("pull reverted the local file onto the abandoned projects/<key>/ copy")
	}
	if strings.Contains(string(after), "project truth") {
		t.Error("the stale project-scoped content came back")
	}
	if m, _, _ := memory.ReadEngram(string(after)); m.Project != "global" {
		t.Errorf("engram.project = %q, want \"global\" — pull reset the scope", m.Project)
	}
}

// dualScopeStore sets up a store holding one id under BOTH global/ and
// projects/<key>/, plus a local copy at the shared base. localProject is what the
// local file records in engram.project — "" for a memory that predates the field.
func dualScopeStore(t *testing.T, localProject string, withProjectCopy bool) (string, string) {
	t.Helper()
	root := t.TempDir()
	hermeticGitEnv(t, root)
	cfg := filepath.Join(root, "gitconfig")
	if err := os.WriteFile(cfg, []byte("[user]\n\tname = D\n\temail = d@example.com\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", cfg)
	bare := filepath.Join(root, "remote.git")
	gitT(t, "", "init", "--bare", bare)
	if err := InitTeam("file://" + bare); err != nil {
		t.Fatalf("InitTeam: %v", err)
	}
	teamDir, _ := Dir()
	key := "github.com/acme/app"

	base := "---\nname: n\n---\n# N\n\nbase\n"
	dig, err := memory.ContentDigest(base)
	if err != nil {
		t.Fatal(err)
	}
	localMem := filepath.Join(root, "proj", "memory")
	if err := os.MkdirAll(localMem, 0o755); err != nil {
		t.Fatal(err)
	}
	local, err := memory.WriteEngram(base, memory.EngramMeta{ID: "DS-1", Scope: "team", Project: localProject, SyncedHash: dig})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(localMem, "n.md"), []byte(local), 0o644); err != nil {
		t.Fatal(err)
	}

	write := func(rel, body, proj string) {
		d, _ := memory.ContentDigest(body)
		raw, err := memory.WriteEngram(body, memory.EngramMeta{ID: "DS-1", Scope: "team", Project: proj, SyncedHash: d})
		if err != nil {
			t.Fatal(err)
		}
		p := filepath.Join(teamDir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(raw), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("global/n.md", "---\nname: n\n---\n# N\n\nglobal advanced\n", "global")
	if withProjectCopy {
		write("projects/"+key+"/n.md", "---\nname: n\n---\n# N\n\nproject advanced\n", key)
	}
	return key, localMem
}

// A dual-scope id must be reconciled by exactly ONE walk. When the local copy
// records no project, both walks used to claim it: the plan counted the file twice
// while the apply wrote it once, so the confirm dialog asked the user to approve a
// number the apply would not deliver — breaking applyPull's own invariant that a
// confirmed plan can never disagree with its apply.
func TestPullDualScopePlanMatchesApply(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	key, localMem := dualScopeStore(t, "", true)
	targets := []ProjectTarget{{Key: key, MemoryDir: localMem}}

	plan, err := PullPlan(targets)
	if err != nil {
		t.Fatalf("PullPlan: %v", err)
	}
	applied, err := PullApply(targets)
	if err != nil {
		t.Fatalf("PullApply: %v", err)
	}
	if plan != applied {
		t.Errorf("plan %+v != apply %+v — one file was counted by both walks", plan, applied)
	}
	if plan.Updated != 1 {
		t.Errorf("Updated = %d, want 1 — a single local file, reconciled once", plan.Updated)
	}
}

// The mirror concern: the guard must not strand a file. A local copy recording a
// scope the store no longer carries has only one store copy to reconcile against,
// so there is no ambiguity to guard — it must still pull, or it badges [behind]
// forever with a dead `p` key.
func TestPullReconcilesWhenRecordedScopeIsGone(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	// local records the project scope, but only the global copy exists in the store
	key, localMem := dualScopeStore(t, "github.com/acme/app", false)

	res, err := Pull([]ProjectTarget{{Key: key, MemoryDir: localMem}})
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if res.Updated != 1 {
		t.Errorf("Updated = %d, want 1 — the only store copy must still reconcile", res.Updated)
	}
	got, err := os.ReadFile(filepath.Join(localMem, "n.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "global advanced") {
		t.Error("the local copy was stranded instead of taking the store's only copy")
	}
}
