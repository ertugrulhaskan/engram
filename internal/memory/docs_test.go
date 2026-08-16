package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

	// A project whose real dir exists on disk, carrying both a CLAUDE.md and the
	// cross-tool AGENTS.md.
	realProjDir = filepath.Join(claudeHome, "code", "app") // -<claudeHome>-code-app decodes here
	write(filepath.Join(realProjDir, "CLAUDE.md"), "# app rules\n")
	write(filepath.Join(realProjDir, "AGENTS.md"), "# app agents\n")
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
	// Claude collapses ".", "_" and "-" all to "-"; mirror that here.
	return strings.NewReplacer(".", "-", "_", "-").Replace(s)
}

func TestDiscoverDocs(t *testing.T) {
	projectsRoot, realProjDir := buildClaudeTree(t)

	docs, err := DiscoverDocs(projectsRoot)
	if err != nil {
		t.Fatal(err)
	}

	// Expect: global CLAUDE.md, app CLAUDE.md, app AGENTS.md, app MEMORY.md,
	// ghost MEMORY.md. CLAUDE.md and AGENTS.md are both DocRules, so they are
	// told apart by Provider — that is the whole point of the second dimension.
	var global, appRules, appAgents, appIndex, ghostIndex bool
	for _, d := range docs {
		switch {
		case d.Scope == "global" && d.Kind == DocRules:
			global = true
		case d.Kind == DocRules && d.Provider == ProviderAgents && d.ProjectDir == realProjDir:
			appAgents = true
		case d.Kind == DocRules && d.Provider == ProviderClaude && d.ProjectDir == realProjDir:
			appRules = true
		case d.Kind == DocIndex && d.ProjectName == filepath.Base(realProjDir):
			appIndex = true
		case d.Kind == DocIndex && d.ProjectName == "gone":
			ghostIndex = true
		}
	}
	if !global || !appRules || !appAgents || !appIndex || !ghostIndex {
		t.Fatalf("missing docs: global=%v appRules=%v appAgents=%v appIndex=%v ghostIndex=%v\n%+v",
			global, appRules, appAgents, appIndex, ghostIndex, docs)
	}

	// The AGENTS.md must carry its own title and body, not inherit CLAUDE.md's.
	for _, d := range docs {
		if d.Provider != ProviderAgents {
			continue
		}
		if d.Title != "AGENTS.md" {
			t.Errorf("AGENTS.md title = %q, want AGENTS.md", d.Title)
		}
		if !strings.Contains(d.Body, "app agents") {
			t.Errorf("AGENTS.md body = %q, want the AGENTS.md contents", d.Body)
		}
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

// TestDocsOrderWithinProject pins the within-scope order: an assistant's own
// rules (CLAUDE.md), then the cross-tool AGENTS.md, then the memory index.
// Without docRank the old two-way comparison left CLAUDE.md/AGENTS.md order
// dependent on read order, which would make the list shuffle between reloads.
func TestDocsOrderWithinProject(t *testing.T) {
	projectsRoot, realProjDir := buildClaudeTree(t)

	docs, err := DiscoverDocs(projectsRoot)
	if err != nil {
		t.Fatal(err)
	}

	var got []string
	for _, d := range docs {
		if d.ProjectDir == realProjDir {
			got = append(got, d.Title)
		}
	}
	want := []string{"CLAUDE.md", "AGENTS.md", "MEMORY.md"}
	if len(got) != len(want) {
		t.Fatalf("project docs = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("project docs = %v, want %v", got, want)
		}
	}
}

// TestDocsSignatureCoversAgentsFile guards the DiscoverDocs/DocsSignature
// lockstep: a file surfaced by the walk but missing from the fingerprint would
// display fine and then never refresh on an external edit — a silent staleness
// bug that no rendering test would catch.
func TestDocsSignatureCoversAgentsFile(t *testing.T) {
	projectsRoot, realProjDir := buildClaudeTree(t)

	before, err := DocsSignature(projectsRoot)
	if err != nil {
		t.Fatal(err)
	}

	// Rewrite AGENTS.md with different content and a distinctly newer modtime.
	agents := filepath.Join(realProjDir, "AGENTS.md")
	if err := os.WriteFile(agents, []byte("# app agents, revised\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	newer := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(agents, newer, newer); err != nil {
		t.Fatal(err)
	}

	after, err := DocsSignature(projectsRoot)
	if err != nil {
		t.Fatal(err)
	}
	if before == after {
		t.Errorf("DocsSignature unchanged after editing AGENTS.md (%q) — the poll reload will miss external edits", before)
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
		filepath.Join(home, "code", "engram.im"),    // single dot in the basename
		filepath.Join(home, "work", "acme.dev"),     // dot in a different segment
		filepath.Join(home, "x", "a.b.c"),           // multiple dots in one name
		filepath.Join(home, "y", "work-acme.io"),    // dot and literal dash together
		filepath.Join(home, "z", "app.engram.im"),   // multi-label, domain-style name
		filepath.Join(home, "w", "work-bigco"),      // literal dash, no dot
		filepath.Join(home, "_clients", "acme-app"), // leading underscore + dashed child
		filepath.Join(home, "u", "svc_app.v2"),      // underscore and dot together
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
