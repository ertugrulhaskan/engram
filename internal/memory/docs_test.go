package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// buildClaudeTree lays out a temp ~/.claude: a global CLAUDE.md, one project
// whose real dir exists (carrying every projectRuleFiles entry) plus a
// MEMORY.md, and one project whose decoded dir does NOT exist (so its rules
// files are unreachable, but its MEMORY.md still shows). Returns the projects
// root to pass to DiscoverDocs.
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

	// A project whose real dir exists on disk, carrying one instruction file per
	// projectRuleFiles entry. Bodies are seeded from the table so a new entry is
	// covered here without editing the fixture.
	realProjDir = filepath.Join(claudeHome, "code", "app") // -<claudeHome>-code-app decodes here
	for _, rf := range projectRuleFiles {
		write(filepath.Join(realProjDir, rf.rel), "# app "+string(rf.provider)+"\n")
	}
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

	// Expect: the global CLAUDE.md, one rules doc per projectRuleFiles entry for
	// the app project, the app MEMORY.md, and the ghost MEMORY.md. Every rules
	// file is DocRules, so they are told apart by Provider — that is the whole
	// point of the second dimension.
	var global, appIndex, ghostIndex bool
	appRules := map[DocProvider]DocFile{}
	for _, d := range docs {
		switch {
		case d.Scope == "global" && d.Kind == DocRules:
			global = true
		case d.Kind == DocRules && d.ProjectDir == realProjDir:
			appRules[d.Provider] = d
		case d.Kind == DocIndex && d.ProjectName == filepath.Base(realProjDir):
			appIndex = true
		case d.Kind == DocIndex && d.ProjectName == "gone":
			ghostIndex = true
		}
	}
	if !global || !appIndex || !ghostIndex {
		t.Fatalf("missing docs: global=%v appIndex=%v ghostIndex=%v\n%+v", global, appIndex, ghostIndex, docs)
	}

	// Each rules file must appear once, under its own provider, carrying its own
	// title and body — never inheriting a sibling's.
	for _, rf := range projectRuleFiles {
		d, ok := appRules[rf.provider]
		if !ok {
			t.Errorf("no doc discovered for %s (provider %q)", rf.rel, rf.provider)
			continue
		}
		if d.Title != rf.title {
			t.Errorf("%s title = %q, want %q", rf.rel, d.Title, rf.title)
		}
		if want := "app " + string(rf.provider); !strings.Contains(d.Body, want) {
			t.Errorf("%s body = %q, want it to contain %q", rf.rel, d.Body, want)
		}
		if want := filepath.Join(realProjDir, rf.rel); d.Path != want {
			t.Errorf("%s path = %q, want %q", rf.rel, d.Path, want)
		}
	}
	if len(appRules) != len(projectRuleFiles) {
		t.Errorf("discovered %d rules docs, want %d: %+v", len(appRules), len(projectRuleFiles), appRules)
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

// TestDocsOrderWithinProject pins the within-scope order: the instruction files
// in projectRuleFiles order (Claude's own rules, then the cross-tool AGENTS.md,
// then the other vendors'), then the memory index last. Without docRank the old
// two-way comparison left that order dependent on read order, which would make
// the list shuffle between reloads.
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
	want := make([]string, 0, len(projectRuleFiles)+1)
	for _, rf := range projectRuleFiles {
		want = append(want, rf.title)
	}
	want = append(want, "MEMORY.md")
	if len(got) != len(want) {
		t.Fatalf("project docs = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("project docs = %v, want %v", got, want)
		}
	}
}

// TestProjectRuleFilesContents pins the table itself. Every other docs test
// drives off projectRuleFiles, which makes them self-consistent but blind in one
// direction: delete a row and they all still pass, just covering less. This is
// the one place that asserts *which* files engram promises to surface, so a
// removed or reordered entry fails loudly.
func TestProjectRuleFilesContents(t *testing.T) {
	want := []ruleFile{
		{"CLAUDE.md", "CLAUDE.md", ProviderClaude},
		{"AGENTS.md", "AGENTS.md", ProviderAgents},
		{"GEMINI.md", "GEMINI.md", ProviderGemini},
		{filepath.Join(".github", "copilot-instructions.md"), "copilot-instructions.md", ProviderCopilot},
	}
	if len(projectRuleFiles) != len(want) {
		t.Fatalf("projectRuleFiles has %d entries, want %d: %+v", len(projectRuleFiles), len(want), projectRuleFiles)
	}
	for i, w := range want {
		if projectRuleFiles[i] != w {
			t.Errorf("projectRuleFiles[%d] = %+v, want %+v", i, projectRuleFiles[i], w)
		}
	}
}

// TestDocsSignatureCoversRuleFiles guards the DiscoverDocs/DocsSignature
// lockstep for every projectRuleFiles entry: a file surfaced by the walk but
// missing from the fingerprint would display fine and then never refresh on an
// external edit — a silent staleness bug that no rendering test would catch.
// Driving off the table means a newly added instruction file is covered here
// the moment it is declared, instead of needing its own hand-written case.
func TestDocsSignatureCoversRuleFiles(t *testing.T) {
	for _, rf := range projectRuleFiles {
		t.Run(rf.rel, func(t *testing.T) {
			projectsRoot, realProjDir := buildClaudeTree(t)

			before, err := DocsSignature(projectsRoot)
			if err != nil {
				t.Fatal(err)
			}

			// Rewrite the file with different content and a distinctly newer modtime.
			path := filepath.Join(realProjDir, rf.rel)
			if err := os.WriteFile(path, []byte("# app "+string(rf.provider)+", revised\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			newer := time.Now().Add(2 * time.Second)
			if err := os.Chtimes(path, newer, newer); err != nil {
				t.Fatal(err)
			}

			after, err := DocsSignature(projectsRoot)
			if err != nil {
				t.Fatal(err)
			}
			if before == after {
				t.Errorf("DocsSignature unchanged after editing %s (%q) — the poll reload will miss external edits", rf.rel, before)
			}
		})
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
