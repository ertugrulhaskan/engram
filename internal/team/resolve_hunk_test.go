package team

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// FirstConflictHunk returns the marker-to-marker block, elides long hunks in
// the middle, and errors when there are no markers.
func TestFirstConflictHunk(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "m.md")

	write := func(s string) {
		t.Helper()
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	write("pre\n<<<<<<< yours (local)\na\nb\n=======\nc\n>>>>>>> team\npost\n")
	hunk, err := FirstConflictHunk(p, 10)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"<<<<<<< yours (local)", "a", "b", "=======", "c", ">>>>>>> team"}
	if strings.Join(hunk, "|") != strings.Join(want, "|") {
		t.Errorf("hunk = %v, want %v", hunk, want)
	}

	// Elision keeps the head and the closing marker.
	write("<<<<<<< yours (local)\n1\n2\n3\n4\n5\n6\n7\n=======\nx\n>>>>>>> team\n")
	hunk, err = FirstConflictHunk(p, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(hunk) != 5 || hunk[3] != "…" || hunk[4] != ">>>>>>> team" {
		t.Errorf("elided hunk = %v", hunk)
	}

	write("no markers here\n")
	if _, err := FirstConflictHunk(p, 5); err == nil {
		t.Error("expected an error for a file without conflict markers")
	}
}
