package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// buildClaudeTree lays out a temp ~/.claude: a global CLAUDE.md, one project
// whose real dir exists (with a CLAUDE.md) plus a MEMORY.md, and one project
// whose decoded dir does NOT exist (so its CLAUDE.md is unreachable, but its
// MEMORY.md still shows). Returns the projects root to pass to DiscoverDocs.
func buildClaudeTree(t *testing.T) (projectsRoot, realProjDir string) {
	t.Helper()
	claudeHome := t.TempDir()
	projectsRoot = filepath.Join(claudeHome, "projects")

	write := func(path, body string) {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Global rules.
	write(filepath.Join(claudeHome, "CLAUDE.md"), "# global rules\n")

	// A project whose real dir exists on disk.
	realProjDir = filepath.Join(claudeHome, "code", "app") // -<claudeHome>-code-app decodes here
	write(filepath.Join(realProjDir, "CLAUDE.md"), "# app rules\n")
	slug := encodeForTest(realProjDir)
	write(filepath.Join(projectsRoot, slug, "memory", "MEMORY.md"), "# app index\n")

	// A project whose decoded dir does not exist (only MEMORY.md is reachable).
	write(filepath.Join(projectsRoot, "-Users-ghost-gone", "memory", "MEMORY.md"), "# ghost index\n")

	return projectsRoot, realProjDir
}

// encodeForTest mirrors Claude Code's project-folder encoding: "/", ".", and any
// literal "-" all collapse to "-". Since dir exists on disk, decodeProjectPath
// round-trips it by walking the filesystem.
func encodeForTest(dir string) string {
	s := strings.ReplaceAll(filepath.ToSlash(dir), "/", "-")
	return strings.ReplaceAll(s, ".", "-")
}

func TestDiscoverDocs(t *testing.T) {
	projectsRoot, realProjDir := buildClaudeTree(t)

	docs, err := DiscoverDocs(projectsRoot)
	if err != nil {
		t.Fatal(err)
	}

	// Expect: global CLAUDE.md, app CLAUDE.md, app MEMORY.md, ghost MEMORY.md.
	var global, appRules, appIndex, ghostIndex bool
	for _, d := range docs {
		switch {
		case d.Scope == "global" && d.Kind == DocRules:
			global = true
		case d.Kind == DocRules && d.ProjectDir == realProjDir:
			appRules = true
		case d.Kind == DocIndex && d.ProjectName == filepath.Base(realProjDir):
			appIndex = true
		case d.Kind == DocIndex && d.ProjectName == "gone":
			ghostIndex = true
		}
	}
	if !global || !appRules || !appIndex || !ghostIndex {
		t.Fatalf("missing docs: global=%v appRules=%v appIndex=%v ghostIndex=%v\n%+v", global, appRules, appIndex, ghostIndex, docs)
	}

	// Global must sort first.
	if docs[0].Scope != "global" {
		t.Errorf("first doc scope = %q, want global", docs[0].Scope)
	}

	// The ghost project's CLAUDE.md must NOT appear (its dir doesn't resolve).
	for _, d := range docs {
		if d.Kind == DocRules && d.ProjectName == "gone" {
			t.Errorf("unexpected CLAUDE.md for unresolved project: %+v", d)
		}
	}
}

// TestDecodeProjectPathDots covers folders whose names contain dots and dashes.
// Claude flattens "/", "." and "-" all to "-", so the decoder must reconstruct
// the real name from disk; without that, a domain-style "app.engram.im" would
// decode to "app/engram/im" and display as just "im". encodeForTest flattens
// dots too, so these cases genuinely exercise the filesystem walk.
func TestDecodeProjectPathDots(t *testing.T) {
	home := t.TempDir()

	cases := []string{
		filepath.Join(home, "code", "engram.im"),           // single dot in the basename
		filepath.Join(home, "work", "acme.dev"),         // dot in a different segment
		filepath.Join(home, "x", "a.b.c"),                  // multiple dots in one name
		filepath.Join(home, "y", "work-acme.io"),       // dot and literal dash together
		filepath.Join(home, "z", "app.engram.im"),          // multi-label, domain-style name
		filepath.Join(home, "w", "work-bigco"), // literal dash, no dot
	}
	for _, dir := range cases {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		got := decodeProjectPath(encodeForTest(dir))
		if got != dir {
			t.Errorf("decodeProjectPath round-trip: got %q, want %q", got, dir)
		}
		if base := filepath.Base(got); base != filepath.Base(dir) {
			t.Errorf("display name: got %q, want %q", base, filepath.Base(dir))
		}
	}

	// Regression guard: an unresolved (deleted/renamed) project can't be probed,
	// so dots aren't recoverable — it falls back to the slash form without panic.
	if got := decodeProjectPath("-Users-ghost-engram-im"); got != "/Users/ghost/engram/im" {
		t.Errorf("unresolved fallback: got %q, want %q", got, "/Users/ghost/engram/im")
	}
}

func TestDocsSignatureChangesOnEdit(t *testing.T) {
	projectsRoot, _ := buildClaudeTree(t)

	sig1, err := DocsSignature(projectsRoot)
	if err != nil {
		t.Fatal(err)
	}
	// Edit the global CLAUDE.md (size changes → signature changes).
	g := filepath.Join(filepath.Dir(projectsRoot), "CLAUDE.md")
	if err := os.WriteFile(g, []byte("# global rules, expanded\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sig2, err := DocsSignature(projectsRoot)
	if err != nil {
		t.Fatal(err)
	}
	if sig1 == sig2 {
		t.Errorf("signature unchanged after editing CLAUDE.md: %q", sig1)
	}
}
