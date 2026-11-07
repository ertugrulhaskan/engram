package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// --- MEMORY.md index lines -------------------------------------------------

// A bullet names the FIRST link that ends the title, a target may hold
// balanced parentheses, and a target that is not a bare .md file in the folder
// (a URL, a subfolder link) is not a memory bullet at all. The old greedy regex
// read "- [A](a.md) — see [B](b.md)" as b.md, cut "notes (1).md" at the first
// ")", and treated URL and subfolder bullets as dangling memories.
func TestIndexLineTargets(t *testing.T) {
	cases := []struct{ line, want string }{
		{"- [A](a.md) — curated, see also [B](b.md)", "a.md"},
		{"- [Title with [brackets]](t.md) — hook", "t.md"},
		{"- [Notes](notes (1).md) — hook", "notes (1).md"},
		{"- [A](./a.md) — hook", "a.md"},
		{"- [A](a.md#section)", "a.md"},
		{"- [A](a%20b.md)", "a%20b.md"},
		{"- [A](a#b.md) — a file with # in its name", "a#b.md"},
		{"- [A](a.md) hook with [B](b.md)", "a.md"},
		{"- [Fix handlers[i](ctx) call](fix.md) — a title with its own ](", "fix.md"},
		{"- [see [B](b.md)](a.md) — a title holding a whole link", "a.md"},
		{"- [A](a.md)", "a.md"},
		{"  - [A](a.md) no separator hook", "a.md"},
		{"- [Docs](https://example.com/guide.md) — external", ""},
		{"- [Sub](sub/s.md) — elsewhere", ""},
		{"- [Img](diagram.png)", ""},
		{"plain text, not a bullet", ""},
	}
	for _, c := range cases {
		if got := lineFile(c.line); got != c.want {
			t.Errorf("lineFile(%q) = %q, want %q", c.line, got, c.want)
		}
	}
}

func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func indexText(t *testing.T, dir string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, IndexFile))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// Upserting b.md must touch only b.md's own bullet — not A's, whose hook links
// to b.md. Before the fix A's line was replaced by B's, leaving two B lines.
func TestUpsertTouchesOnlyItsOwnBullet(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"a.md":    "---\nname: a\ndescription: about a\n---\n# A\n",
		"b.md":    "---\nname: b\ndescription: about b\n---\n# B\n",
		IndexFile: "# Memory index\n\n- [A](a.md) — curated, see also [B](b.md)\n- [B](b.md) — old b\n",
	})
	if err := UpsertIndex(dir, "b.md"); err != nil {
		t.Fatal(err)
	}
	got := indexText(t, dir)
	if !strings.Contains(got, "- [A](a.md) — curated, see also [B](b.md)") {
		t.Errorf("A's bullet was rewritten:\n%s", got)
	}
	if n := strings.Count(got, "](b.md) — about b"); n != 1 {
		t.Errorf("want exactly one refreshed B bullet, got %d:\n%s", n, got)
	}
}

// Repeated upserts of an unusual name must not append duplicates, and a
// reconcile must find nothing to do: parentheses, "#" and "%" all round-trip.
func TestUpsertUnusualNamesDoNotDuplicate(t *testing.T) {
	for _, name := range []string{"notes (1).md", "notes (draft.md", "a).md", "a#b.md", "a%20b.md"} {
		dir := t.TempDir()
		writeFiles(t, dir, map[string]string{name: "---\nname: n\n---\n# N\n"})
		for i := 0; i < 3; i++ {
			if err := UpsertIndex(dir, name); err != nil {
				t.Fatal(err)
			}
		}
		if n := strings.Count(indexText(t, dir), "]("+name+")"); n != 1 {
			t.Errorf("%s: want one bullet after three upserts, got %d:\n%s", name, n, indexText(t, dir))
		}
		if un, dang, _ := IndexDrift(dir); len(un)+len(dang) != 0 {
			t.Errorf("%s: drift after upsert: unindexed=%v dangling=%v", name, un, dang)
		}
		if added, removed, _ := ReconcileIndex(dir); added+removed != 0 {
			t.Errorf("%s: reconcile changed the index (added %d, removed %d)", name, added, removed)
		}
	}
}

// Reconcile runs after every pull, unprompted. It must leave alone bullets that
// are not memories (a URL, a subfolder link) and must count ./a.md as a.md —
// before the fix it deleted all three and re-added a.md with a rebuilt hook.
func TestReconcileLeavesNonMemoryBulletsAlone(t *testing.T) {
	dir := t.TempDir()
	index := "# Memory index\n\n- [A](./a.md) — curated hook for a\n" +
		"- [Guide](https://example.com/guide.md) — external reading\n" +
		"- [Sub](sub/s.md) — a note kept elsewhere\n"
	writeFiles(t, dir, map[string]string{
		"a.md":    "---\nname: a\ndescription: rebuilt\n---\n# A\n",
		IndexFile: index,
	})
	added, removed, err := ReconcileIndex(dir)
	if err != nil {
		t.Fatal(err)
	}
	if added != 0 || removed != 0 {
		t.Errorf("reconcile added %d, removed %d; want 0, 0", added, removed)
	}
	if got := indexText(t, dir); got != index {
		t.Errorf("reconcile rewrote the index:\n%s", got)
	}
}

// --- writes ---------------------------------------------------------------

// MEMORY.md is written atomically (temp + rename), so the rename must not
// replace a symlinked index with a plain file — it writes through to the target.
func TestIndexWriteKeepsSymlinkedIndex(t *testing.T) {
	dir, shared := t.TempDir(), t.TempDir()
	target := filepath.Join(shared, IndexFile)
	writeFiles(t, shared, map[string]string{IndexFile: "# Memory index\n"})
	if err := os.Symlink(target, filepath.Join(dir, IndexFile)); err != nil {
		t.Fatal(err)
	}
	writeFiles(t, dir, map[string]string{"a.md": "---\nname: a\n---\n# A\n"})
	if err := UpsertIndex(dir, "a.md"); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Lstat(filepath.Join(dir, IndexFile)); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("the symlinked MEMORY.md was replaced by a plain file (err=%v)", err)
	}
	if b, _ := os.ReadFile(target); !strings.Contains(string(b), "](a.md)") {
		t.Errorf("the write did not reach the link's target:\n%s", b)
	}
}

// Create must not follow a dangling symlink at the chosen name and write
// outside the memory dir; it picks the next free name instead.
func TestCreateDoesNotWriteThroughDanglingSymlink(t *testing.T) {
	dir, outside := t.TempDir(), t.TempDir()
	victim := filepath.Join(outside, "target.md")
	if err := os.Symlink(victim, filepath.Join(dir, "notes.md")); err != nil {
		t.Fatal(err)
	}
	p, err := Create(dir, "notes")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(p) != "notes-2.md" {
		t.Errorf("Create chose %s, want notes-2.md", filepath.Base(p))
	}
	if _, err := os.Stat(victim); err == nil {
		t.Error("Create wrote through the dangling symlink, outside the memory dir")
	}
}

// --- discovery and polling --------------------------------------------------

// The poll must notice an edit made through a symlinked memory file. It used
// the link's own mtime and size, so the TUI never reloaded such an edit.
func TestSignatureSeesEditThroughSymlinkedMemory(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	mem := filepath.Join(root, "-x-proj", "memory")
	if err := os.MkdirAll(mem, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(outside, "a.md")
	writeFiles(t, outside, map[string]string{"a.md": "---\nname: a\n---\nv1\n"})
	if err := os.Symlink(target, filepath.Join(mem, "a.md")); err != nil {
		t.Fatal(err)
	}
	before, err := Signature(root)
	if err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(2 * time.Second)
	if err := os.WriteFile(target, []byte("---\nname: a\n---\nv2, longer\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_ = os.Chtimes(target, later, later)
	after, _ := Signature(root)
	if after == before {
		t.Error("Signature did not change after an edit through the symlink")
	}
}

// A project folder under ~/.claude/projects may itself be a symlink (a moved
// project kept under its old key); Discover must find its memories.
func TestDiscoverFollowsSymlinkedProjectFolder(t *testing.T) {
	root, real := t.TempDir(), t.TempDir()
	if err := os.MkdirAll(filepath.Join(real, "memory"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFiles(t, filepath.Join(real, "memory"), map[string]string{"a.md": "---\nname: a\n---\n# A\n"})
	if err := os.Symlink(real, filepath.Join(root, "-x-moved")); err != nil {
		t.Fatal(err)
	}
	mems, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(mems) != 1 {
		t.Errorf("Discover found %d memories under a symlinked project folder, want 1", len(mems))
	}
}

// claudeEncode is Claude Code's own project-folder rule, read from its binary
// (2.1.x): every UTF-16 code unit outside [a-zA-Z0-9] becomes "-", so an astral
// character (an emoji) becomes two.
func claudeEncode(p string) string {
	var b strings.Builder
	for _, r := range p {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r > 0xFFFF:
			b.WriteString("--")
		default:
			b.WriteByte('-')
		}
	}
	return b.String()
}

// The decoder must undo that rule for every character it flattens — not just
// ".", "_" and "-". A path with a space, an accent, "@", "+" or "~" decoded to
// a made-up path, hiding the project's name and its rule files.
func TestDecodeProjectPathAnySeparator(t *testing.T) {
	base := t.TempDir()
	for _, name := range []string{"My Projects/app", "café/x@y+z", "home~old/proj 2026"} {
		real := filepath.Join(base, filepath.FromSlash(name))
		if err := os.MkdirAll(real, 0o755); err != nil {
			t.Fatal(err)
		}
		if got := decodeProjectPath(claudeEncode(real)); got != real {
			t.Errorf("decode(%s) = %s, want %s", claudeEncode(real), got, real)
		}
	}
}

// A writable MEMORY.md in a directory engram can't create files in (a symlink
// into a read-only checkout) must still be updated, as os.WriteFile did.
func TestIndexWriteFallsBackInAReadOnlyDir(t *testing.T) {
	dir, shared := t.TempDir(), t.TempDir()
	target := filepath.Join(shared, IndexFile)
	writeFiles(t, shared, map[string]string{IndexFile: "# Memory index\n"})
	if err := os.Symlink(target, filepath.Join(dir, IndexFile)); err != nil {
		t.Fatal(err)
	}
	writeFiles(t, dir, map[string]string{"a.md": "---\nname: a\n---\n# A\n"})
	if err := os.Chmod(shared, 0o555); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(shared, 0o755)
	if err := UpsertIndex(dir, "a.md"); err != nil {
		t.Fatalf("UpsertIndex in a read-only dir: %v", err)
	}
	if b, _ := os.ReadFile(target); !strings.Contains(string(b), "](a.md)") {
		t.Errorf("the index was not updated:\n%s", b)
	}
}

// A MEMORY.md the user made read-only must not be replaced by the rename.
func TestIndexWriteRespectsAReadOnlyIndex(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"a.md": "---\nname: a\n---\n# A\n", IndexFile: "# Memory index\n"})
	idx := filepath.Join(dir, IndexFile)
	if err := os.Chmod(idx, 0o444); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(idx, 0o644)
	if err := UpsertIndex(dir, "a.md"); err == nil {
		t.Error("UpsertIndex replaced a read-only MEMORY.md; want a permission error")
	}
	if b, _ := os.ReadFile(idx); strings.Contains(string(b), "a.md") {
		t.Errorf("the read-only index was modified:\n%s", b)
	}
}

// A hard-linked MEMORY.md is written in place, so every link sees the update.
func TestIndexWriteKeepsHardLinks(t *testing.T) {
	dir, other := t.TempDir(), t.TempDir()
	writeFiles(t, dir, map[string]string{"a.md": "---\nname: a\n---\n# A\n", IndexFile: "# Memory index\n"})
	link := filepath.Join(other, "MEMORY-link.md")
	if err := os.Link(filepath.Join(dir, IndexFile), link); err != nil {
		t.Skipf("hard links unavailable: %v", err)
	}
	if err := UpsertIndex(dir, "a.md"); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(link); !strings.Contains(string(b), "](a.md)") {
		t.Errorf("the other hard link kept the old content:\n%s", b)
	}
}
