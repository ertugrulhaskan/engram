package plan

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func write(t *testing.T, dir, name, content string, mod time.Time) {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if !mod.IsZero() {
		if err := os.Chtimes(p, mod, mod); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDiscover(t *testing.T) {
	dir := t.TempDir()
	old := time.Now().Add(-48 * time.Hour)
	recent := time.Now().Add(-1 * time.Hour)
	write(t, dir, "alpha.md", "# Plan: Build the thing\n\nbody\n", old)
	write(t, dir, "beta.md", "# Just a heading\n\nbody\n", recent)
	write(t, dir, "no-heading-here.md", "first line, not a heading\n", time.Time{})
	write(t, dir, "ignore.txt", "not markdown\n", time.Time{})

	plans, err := Discover(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(plans) != 3 {
		t.Fatalf("got %d plans, want 3 (txt excluded)", len(plans))
	}
	// Newest first: beta (recent) before alpha (old). no-heading has ~now mtime.
	byTitle := map[string]Plan{}
	for _, p := range plans {
		byTitle[p.Title] = p
	}
	if _, ok := byTitle["Build the thing"]; !ok { // "# Plan:" prefix stripped
		t.Errorf("title prefix not stripped; got titles %v", titles(plans))
	}
	if _, ok := byTitle["Just a heading"]; !ok {
		t.Errorf("heading title missing; got %v", titles(plans))
	}
	if _, ok := byTitle["No Heading Here"]; !ok { // filename fallback, titleized
		t.Errorf("filename fallback missing; got %v", titles(plans))
	}
	// Ordering: alpha (oldest) must come after beta.
	ai, bi := indexOf(plans, "Build the thing"), indexOf(plans, "Just a heading")
	if !(bi < ai) {
		t.Errorf("expected newest-first (beta before alpha); order=%v", titles(plans))
	}

	// Missing dir → empty, no error.
	if ps, err := Discover(filepath.Join(dir, "nope")); err != nil || len(ps) != 0 {
		t.Errorf("missing dir: got (%d plans, %v), want (0, nil)", len(ps), err)
	}
}

func TestSignatureChanges(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "a.md", "# Plan: A\n", time.Time{})
	s1, err := Signature(dir)
	if err != nil {
		t.Fatal(err)
	}
	if s2, _ := Signature(dir); s2 != s1 {
		t.Fatalf("not deterministic: %q != %q", s1, s2)
	}
	write(t, dir, "b.md", "# Plan: B\n", time.Time{})
	if s2, _ := Signature(dir); s2 == s1 {
		t.Error("adding a plan did not change the signature")
	}
}

func titles(plans []Plan) []string {
	out := make([]string, len(plans))
	for i, p := range plans {
		out[i] = p.Title
	}
	return out
}

func indexOf(plans []Plan, title string) int {
	for i, p := range plans {
		if p.Title == title {
			return i
		}
	}
	return -1
}
