package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readIndex(t *testing.T, memDir string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(memDir, IndexFile))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func memDirWith(t *testing.T, files map[string]string) string {
	t.Helper()
	memDir := filepath.Join(t.TempDir(), "memory")
	if err := os.MkdirAll(memDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(memDir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return memDir
}

func TestUpsertIndexCreatesAndAppends(t *testing.T) {
	memDir := memDirWith(t, map[string]string{
		"one.md": "---\nname: one\ndescription: first hook\nmetadata:\n  type: project\n---\n# One\n",
		"two.md": "# Two\n\nsecond para becomes the hook\n",
	})

	// No index yet → created with a header.
	if err := UpsertIndex(memDir, "one.md"); err != nil {
		t.Fatal(err)
	}
	idx := readIndex(t, memDir)
	if !strings.HasPrefix(idx, indexHeader) {
		t.Errorf("index missing header:\n%s", idx)
	}
	if !strings.Contains(idx, "- [One](one.md) — first hook") {
		t.Errorf("one.md bullet wrong:\n%s", idx)
	}

	// Second upsert appends a new bullet, keeping the first.
	if err := UpsertIndex(memDir, "two.md"); err != nil {
		t.Fatal(err)
	}
	idx = readIndex(t, memDir)
	if !strings.Contains(idx, "- [One](one.md)") || !strings.Contains(idx, "- [Two](two.md) — second para becomes the hook") {
		t.Errorf("append failed:\n%s", idx)
	}
	if got := strings.Count(idx, "(one.md)"); got != 1 {
		t.Errorf("one.md duplicated %d times:\n%s", got, idx)
	}
}

func TestUpsertIndexReplacesInPlacePreservingOrder(t *testing.T) {
	memDir := memDirWith(t, map[string]string{
		"a.md": "# A\n\nhook a\n",
		"b.md": "# B\n\nhook b\n",
		"c.md": "# C\n\nhook c\n",
	})
	// Hand-author an index in a deliberate (non-alphabetical) order.
	manual := indexHeader + "\n\n" +
		"- [C](c.md) — hook c\n" +
		"- [A](a.md) — hook a\n" +
		"- [B](b.md) — old hook b\n"
	if err := os.WriteFile(filepath.Join(memDir, IndexFile), []byte(manual), 0o644); err != nil {
		t.Fatal(err)
	}
	// Edit b.md's content, then upsert — its line must update in place, order kept.
	if err := os.WriteFile(filepath.Join(memDir, "b.md"), []byte("# B\n\nbrand new hook b\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := UpsertIndex(memDir, "b.md"); err != nil {
		t.Fatal(err)
	}
	idx := readIndex(t, memDir)
	lines := []string{}
	for _, ln := range strings.Split(idx, "\n") {
		if f := lineFile(ln); f != "" {
			lines = append(lines, f)
		}
	}
	want := []string{"c.md", "a.md", "b.md"}
	if strings.Join(lines, ",") != strings.Join(want, ",") {
		t.Errorf("order changed: got %v want %v", lines, want)
	}
	if !strings.Contains(idx, "- [B](b.md) — brand new hook b") || strings.Contains(idx, "old hook b") {
		t.Errorf("b.md not updated in place:\n%s", idx)
	}
}

func TestRemoveIndex(t *testing.T) {
	memDir := memDirWith(t, map[string]string{"a.md": "# A\n", "b.md": "# B\n"})
	manual := indexHeader + "\n\n- [A](a.md) — x\n- [B](b.md) — y\n"
	os.WriteFile(filepath.Join(memDir, IndexFile), []byte(manual), 0o644)

	if err := RemoveIndex(memDir, "a.md"); err != nil {
		t.Fatal(err)
	}
	idx := readIndex(t, memDir)
	if strings.Contains(idx, "(a.md)") || !strings.Contains(idx, "(b.md)") {
		t.Errorf("remove failed:\n%s", idx)
	}
	// Missing index is a no-op.
	empty := memDirWith(t, nil)
	if err := RemoveIndex(empty, "nope.md"); err != nil {
		t.Errorf("remove on missing index should be nil, got %v", err)
	}
}

func TestIndexDriftAndReconcile(t *testing.T) {
	memDir := memDirWith(t, map[string]string{
		"present.md": "# Present\n\nhook\n",
		"added.md":   "# Added\n\nnew\n", // on disk, not in index
	})
	// Index references present.md and a gone.md that doesn't exist.
	manual := indexHeader + "\n\n- [Present](present.md) — hook\n- [Gone](gone.md) — stale\n"
	os.WriteFile(filepath.Join(memDir, IndexFile), []byte(manual), 0o644)

	unindexed, dangling, err := IndexDrift(memDir)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(unindexed, ",") != "added.md" {
		t.Errorf("unindexed = %v, want [added.md]", unindexed)
	}
	if strings.Join(dangling, ",") != "gone.md" {
		t.Errorf("dangling = %v, want [gone.md]", dangling)
	}

	added, removed, err := ReconcileIndex(memDir)
	if err != nil {
		t.Fatal(err)
	}
	if added != 1 || removed != 1 {
		t.Errorf("reconcile counts = (+%d −%d), want (+1 −1)", added, removed)
	}
	idx := readIndex(t, memDir)
	if !strings.Contains(idx, "(added.md)") || strings.Contains(idx, "(gone.md)") || !strings.Contains(idx, "(present.md)") {
		t.Errorf("reconcile wrong:\n%s", idx)
	}
	// Now in sync.
	un, dang, _ := IndexDrift(memDir)
	if len(un) != 0 || len(dang) != 0 {
		t.Errorf("still drifting after reconcile: un=%v dang=%v", un, dang)
	}
}
