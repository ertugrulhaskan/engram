package plan

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// The poll must notice an edit made through a symlinked plan file. Signature
// used the link's own mtime and size, so such an edit never reloaded.
func TestSignatureSeesEditThroughSymlinkedPlan(t *testing.T) {
	dir, outside := t.TempDir(), t.TempDir()
	target := filepath.Join(outside, "p.md")
	if err := os.WriteFile(target, []byte("# Plan\nv1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(dir, "p.md")); err != nil {
		t.Fatal(err)
	}
	before, err := Signature(dir)
	if err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(2 * time.Second)
	if err := os.WriteFile(target, []byte("# Plan\nv2, longer\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_ = os.Chtimes(target, later, later)
	if after, _ := Signature(dir); after == before {
		t.Error("Signature did not change after an edit through the symlink")
	}
}
