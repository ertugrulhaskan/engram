package memory

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeMem(t *testing.T, root, proj, name, content string) string {
	t.Helper()
	dir := filepath.Join(root, proj, "memory")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestSignature(t *testing.T) {
	root := t.TempDir()
	writeMem(t, root, "-proj-a", "one.md", "# One\n\nbody\n")
	writeMem(t, root, "-proj-a", "MEMORY.md", "- [One](one.md) — hook\n")

	sig1, err := Signature(root)
	if err != nil {
		t.Fatal(err)
	}
	if sig1b, _ := Signature(root); sig1b != sig1 {
		t.Fatalf("not deterministic: %q != %q", sig1, sig1b)
	}

	// Adding a memory changes the signature.
	writeMem(t, root, "-proj-a", "two.md", "# Two\n\nbody\n")
	sig2, _ := Signature(root)
	if sig2 == sig1 {
		t.Error("adding a file did not change the signature")
	}

	// Editing content (size differs) changes the signature.
	writeMem(t, root, "-proj-a", "one.md", "# One\n\nbody is longer now\n")
	sig3, _ := Signature(root)
	if sig3 == sig2 {
		t.Error("editing a file did not change the signature")
	}

	// A modtime-only change (same size) changes the signature.
	future := time.Now().Add(2 * time.Hour)
	if err := os.Chtimes(filepath.Join(root, "-proj-a", "memory", "two.md"), future, future); err != nil {
		t.Fatal(err)
	}
	sig4, _ := Signature(root)
	if sig4 == sig3 {
		t.Error("modtime change did not change the signature")
	}

	// MEMORY.md is included, so index edits count too.
	writeMem(t, root, "-proj-a", "MEMORY.md", "- [One](one.md) — a different hook\n")
	if sig5, _ := Signature(root); sig5 == sig4 {
		t.Error("MEMORY.md edit did not change the signature")
	}
}
