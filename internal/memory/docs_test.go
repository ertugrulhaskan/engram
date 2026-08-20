package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// buildClaudeTree lays out a temp home dir: a ~/.claude with a global CLAUDE.md,
// one project whose real dir exists (carrying every projectRuleFiles entry) plus
// a MEMORY.md, and one project whose decoded dir does NOT exist (so its rules
// files are unreachable, but its MEMORY.md still shows). Returns the projects
// root to pass to DiscoverDocs.
//
// The nesting matters: claudeLayout walks up from the projects root to ~/.claude
// and again to the home dir, so the fixture must be <tmp>/.claude/projects. Root
// the tree one level shallower and the globalRuleFiles lookups escape the temp
// dir and read the developer's real ~/.gemini/GEMINI.md.
func buildClaudeTree(t *testing.T) (projectsRoot, realProjDir, home string) {
	t.Helper()
	home = t.TempDir()
	claudeHome := filepath.Join(home, ".claude")
	projectsRoot = filepath.Join(claudeHome, "projects")

	write := func(path, body string) {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Global rules: Claude's own, plus one file per globalRuleFiles entry in the
	// home dir. Seeded from the table so a new vendor is covered without editing
	// the fixture — the same contract projectRuleFiles has below.
	write(filepath.Join(claudeHome, "CLAUDE.md"), "# global rules\n")
	for _, rf := range globalRuleFiles {
		write(filepath.Join(home, rf.rel), "# global "+string(rf.provider)+"\n")
	}

	// A project whose real dir exists on disk, carrying one instruction file per
	// projectRuleFiles entry. Bodies are seeded from the table so a new entry is
	// covered here without editing the fixture.
	realProjDir = filepath.Join(home, "code", "app") // -<home>-code-app decodes here
	for _, rf := range projectRuleFiles {
		write(filepath.Join(realProjDir, rf.rel), "# app "+string(rf.provider)+"\n")
	}
	slug := encodeForTest(realProjDir)
	write(filepath.Join(projectsRoot, slug, "memory", "MEMORY.md"), "# app index\n")

	// A project whose decoded dir does not exist (only MEMORY.md is reachable).
	write(filepath.Join(projectsRoot, "-Users-ghost-gone", "memory", "MEMORY.md"), "# ghost index\n")

	return projectsRoot, realProjDir, home
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
	projectsRoot, realProjDir, _ := buildClaudeTree(t)

	docs, err := DiscoverDocs(projectsRoot, nil)
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
	projectsRoot, realProjDir, _ := buildClaudeTree(t)

	docs, err := DiscoverDocs(projectsRoot, nil)
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
			projectsRoot, realProjDir, _ := buildClaudeTree(t)

			before, err := DocsSignature(projectsRoot, nil)
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

			after, err := DocsSignature(projectsRoot, nil)
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
	projectsRoot, _, _ := buildClaudeTree(t)

	sig1, err := DocsSignature(projectsRoot, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Edit the global CLAUDE.md (size changes → signature changes).
	g := filepath.Join(filepath.Dir(projectsRoot), "CLAUDE.md")
	if err := os.WriteFile(g, []byte("# global rules, expanded\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sig2, err := DocsSignature(projectsRoot, nil)
	if err != nil {
		t.Fatal(err)
	}
	if sig1 == sig2 {
		t.Errorf("signature unchanged after editing CLAUDE.md: %q", sig1)
	}
}

// TestGlobalRuleFilesContents pins the global table the way
// TestProjectRuleFilesContents pins the project one. Every other global test
// drives off globalRuleFiles, so deleting a row would leave them all passing
// while silently covering less. This is the one place that asserts *which*
// home-dir files engram promises to surface.
func TestGlobalRuleFilesContents(t *testing.T) {
	want := []ruleFile{
		{filepath.Join(".codex", "AGENTS.md"), "AGENTS.md", ProviderAgents},
		{filepath.Join(".gemini", "GEMINI.md"), "GEMINI.md", ProviderGemini},
	}
	if len(globalRuleFiles) != len(want) {
		t.Fatalf("globalRuleFiles has %d entries, want %d: %+v", len(globalRuleFiles), len(want), globalRuleFiles)
	}
	for i, w := range want {
		if globalRuleFiles[i] != w {
			t.Errorf("globalRuleFiles[%d] = %+v, want %+v", i, globalRuleFiles[i], w)
		}
	}
}

// TestDiscoverDocsFindsVendorGlobals is the point of globalRuleFiles: a vendor's
// home-dir instruction file applies to every project, so it belongs in /files
// next to ~/.claude/CLAUDE.md. It must land in the global scope carrying no
// project — an empty ProjectDir is what tells the TUI there is no repo to open,
// so a wrong value here would point @Claude at an unrelated project.
func TestDiscoverDocsFindsVendorGlobals(t *testing.T) {
	projectsRoot, _, home := buildClaudeTree(t)

	docs, err := DiscoverDocs(projectsRoot, nil)
	if err != nil {
		t.Fatal(err)
	}

	byProvider := map[DocProvider]DocFile{}
	for _, d := range docs {
		if d.Scope == "global" {
			byProvider[d.Provider] = d
		}
	}
	if _, ok := byProvider[ProviderClaude]; !ok {
		t.Error("the global CLAUDE.md stopped being discovered")
	}
	for _, rf := range globalRuleFiles {
		d, ok := byProvider[rf.provider]
		if !ok {
			t.Errorf("no global doc discovered for %s (provider %q)", rf.rel, rf.provider)
			continue
		}
		if d.Title != rf.title {
			t.Errorf("%s title = %q, want %q", rf.rel, d.Title, rf.title)
		}
		if want := filepath.Join(home, rf.rel); d.Path != want {
			t.Errorf("%s path = %q, want %q", rf.rel, d.Path, want)
		}
		if want := "global " + string(rf.provider); !strings.Contains(d.Body, want) {
			t.Errorf("%s body = %q, want it to contain %q", rf.rel, d.Body, want)
		}
		if d.Kind != DocRules {
			t.Errorf("%s kind = %q, want %q", rf.rel, d.Kind, DocRules)
		}
		if d.ProjectName != "" || d.ProjectDir != "" || d.MemoryDir != "" {
			t.Errorf("%s belongs to no project, got name=%q dir=%q mem=%q",
				rf.rel, d.ProjectName, d.ProjectDir, d.MemoryDir)
		}
	}

	// Every global doc sorts ahead of every project doc, and within the global
	// scope they follow docRank (Claude's own rules, then the other vendors').
	var globals []string
	seenProject := false
	for _, d := range docs {
		if d.Scope == "global" {
			if seenProject {
				t.Errorf("global doc %q sorted after a project doc", d.Title)
			}
			globals = append(globals, string(d.Provider))
			continue
		}
		seenProject = true
	}
	want := []string{string(ProviderClaude)}
	for _, rf := range globalRuleFiles {
		want = append(want, string(rf.provider))
	}
	if len(globals) != len(want) {
		t.Fatalf("global docs = %v, want %v", globals, want)
	}
	for i := range want {
		if globals[i] != want[i] {
			t.Fatalf("global docs = %v, want %v", globals, want)
		}
	}
}

// TestDocsSignatureCoversGlobalRuleFiles is the lockstep guard for the global
// table, matching TestDocsSignatureCoversRuleFiles for the project one. A global
// file surfaced by the walk but missing from the fingerprint would render on
// launch and then never refresh — and a *global* file is the one most likely to
// be edited outside engram, since every project sees it.
func TestDocsSignatureCoversGlobalRuleFiles(t *testing.T) {
	for _, rf := range globalRuleFiles {
		t.Run(rf.rel, func(t *testing.T) {
			projectsRoot, _, home := buildClaudeTree(t)

			before, err := DocsSignature(projectsRoot, nil)
			if err != nil {
				t.Fatal(err)
			}

			path := filepath.Join(home, rf.rel)
			if err := os.WriteFile(path, []byte("# global "+string(rf.provider)+", revised\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			newer := time.Now().Add(2 * time.Second)
			if err := os.Chtimes(path, newer, newer); err != nil {
				t.Fatal(err)
			}

			after, err := DocsSignature(projectsRoot, nil)
			if err != nil {
				t.Fatal(err)
			}
			if before == after {
				t.Errorf("DocsSignature unchanged after editing %s (%q) — the poll reload will miss external edits", rf.rel, before)
			}
		})
	}
}

// TestDiscoverDocsStaysInsideTheFixtureHome guards the trap that reading the
// home dir introduced. claudeLayout derives home by walking up twice from the
// projects root, so a fixture rooted one level too shallow resolves home to the
// directory holding the temp tree — and the run would read the developer's own
// ~/.gemini/GEMINI.md, passing or failing based on their machine. Pinning every
// discovered path inside the fixture keeps that non-hermetic version from
// creeping back in.
func TestDiscoverDocsStaysInsideTheFixtureHome(t *testing.T) {
	projectsRoot, _, home := buildClaudeTree(t)

	docs, err := DiscoverDocs(projectsRoot, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) == 0 {
		t.Fatal("fixture produced no docs")
	}
	for _, d := range docs {
		rel, err := filepath.Rel(home, d.Path)
		if err != nil || strings.HasPrefix(rel, "..") {
			t.Errorf("doc %q lies outside the fixture home %q — the walk escaped into the real filesystem", d.Path, home)
		}
	}
}

// TestGlobalRuleFilesSurviveMissingClaudeHome pins the one claim the README,
// SPEC, ROADMAP and changelog all make about these files: unlike per-project
// discovery, they are read straight from the home dir, so they appear with no
// ~/.claude tree and no scanRoots configured. That is the whole reason they are
// a separate table rather than another scan root — scan roots find *projects*,
// and these belong to none. Four doc surfaces assert it; this is what makes it
// true rather than aspirational.
func TestGlobalRuleFilesSurviveMissingClaudeHome(t *testing.T) {
	home := t.TempDir()
	for _, rf := range globalRuleFiles {
		path := filepath.Join(home, rf.rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("# global "+string(rf.provider)+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	missing := filepath.Join(home, ".claude", "projects") // never created

	docs, err := DiscoverDocs(missing, nil)
	if err != nil {
		t.Fatalf("DiscoverDocs with no ~/.claude = %v, want the vendors' global files", err)
	}
	if len(docs) != len(globalRuleFiles) {
		t.Fatalf("docs = %+v, want exactly the %d globalRuleFiles entries", docs, len(globalRuleFiles))
	}
	for i, rf := range globalRuleFiles {
		if docs[i].Provider != rf.provider || docs[i].Scope != "global" {
			t.Errorf("docs[%d] = provider %q scope %q, want %q/global", i, docs[i].Provider, docs[i].Scope, rf.provider)
		}
	}

	// The fingerprint must cover them too, or an external edit never reloads.
	before, err := DocsSignature(missing, nil)
	if err != nil {
		t.Fatal(err)
	}
	if before == "" {
		t.Fatal("DocsSignature = \"\" with no ~/.claude; want the hash covering the vendors' global files")
	}
	edited := filepath.Join(home, globalRuleFiles[0].rel)
	if err := os.WriteFile(edited, []byte("# revised\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	newer := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(edited, newer, newer); err != nil {
		t.Fatal(err)
	}
	after, err := DocsSignature(missing, nil)
	if err != nil {
		t.Fatal(err)
	}
	if before == after {
		t.Error("DocsSignature unchanged after editing a global file with no ~/.claude — the poll reload will miss it")
	}
}

// TestProjectRuleDirsContents pins the rule-dir table itself. The discovery,
// signature, and scan-root tests all drive off it, so they would keep passing
// while covering less if an entry were dropped or a suffix loosened — this is
// the same contract TestProjectRuleFilesContents holds for the flat table.
func TestProjectRuleDirsContents(t *testing.T) {
	want := []ruleDir{
		{filepath.Join(".cursor", "rules"), ".mdc", ProviderCursor},
		{filepath.Join(".github", "instructions"), ".instructions.md", ProviderCopilot},
	}
	if len(projectRuleDirs) != len(want) {
		t.Fatalf("projectRuleDirs has %d entries, want %d: %+v", len(projectRuleDirs), len(want), projectRuleDirs)
	}
	for i, w := range want {
		if projectRuleDirs[i] != w {
			t.Errorf("projectRuleDirs[%d] = %+v, want %+v", i, projectRuleDirs[i], w)
		}
	}
}

// TestRuleDetail pins the one-line scoping note built from a rule file's
// frontmatter — including the YAML-hostile plain scalar (`globs: **/*.ts`
// starts with an alias character) that is exactly why the keys are read
// line-wise instead of through yaml.Unmarshal.
func TestRuleDetail(t *testing.T) {
	cases := []struct {
		name, fm, want string
	}{
		{"globs, YAML-invalid scalar", "globs: **/*.ts\nalwaysApply: false", "applies to **/*.ts"},
		{"applyTo, quoted", `applyTo: "app/models/**/*.rb"`, "applies to app/models/**/*.rb"},
		{"alwaysApply", "alwaysApply: true", "always applied"},
		{"description fallback", "description: Frontend standards\nalwaysApply: false", "Frontend standards"},
		{"globs beats description", "description: d\nglobs: src/**", "applies to src/**"},
		{"nothing set", "alwaysApply: false", ""},
	}
	for _, c := range cases {
		if got := ruleDetail(c.fm); got != c.want {
			t.Errorf("%s: ruleDetail = %q, want %q", c.name, got, c.want)
		}
	}
}

// TestDiscoverDocsFindsRuleDirFiles covers the rule directories end to end:
// found (including in a nested folder, which Cursor documents), suffix-strict
// (a plain .md inside .cursor/rules is ignored, as Cursor itself ignores it),
// frontmatter split off the body with the scoping surfaced as Detail, and
// sorted after the fixed instruction files but before MEMORY.md.
func TestDiscoverDocsFindsRuleDirFiles(t *testing.T) {
	projectsRoot, realProjDir, _ := buildClaudeTree(t)
	write := func(rel, body string) {
		path := filepath.Join(realProjDir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(".cursor", "rules", "api.mdc"),
		"---\nglobs: **/*.ts\nalwaysApply: false\n---\n\nUse named exports.\n")
	write(filepath.Join(".cursor", "rules", "frontend", "components.mdc"),
		"---\nalwaysApply: true\n---\n\nCopyright header everywhere.\n")
	write(filepath.Join(".cursor", "rules", "notes.md"), "not a rule — wrong extension\n")
	write(filepath.Join(".github", "instructions", "py.instructions.md"),
		"---\napplyTo: \"**/*.py\"\n---\n\nFollow PEP 8.\n")

	docs, err := DiscoverDocs(projectsRoot, nil)
	if err != nil {
		t.Fatal(err)
	}
	byTitle := map[string]DocFile{}
	pos := map[string]int{}
	for i, d := range docs {
		if d.Scope != "global" { // one project in the fixture — titles are unique
			byTitle[d.Title] = d
			pos[d.Title] = i
		}
	}

	api, ok := byTitle["api.mdc"]
	if !ok {
		t.Fatalf("api.mdc not discovered; project docs: %v", pos)
	}
	if api.Provider != ProviderCursor || api.Kind != DocRules {
		t.Errorf("api.mdc provider/kind = %s/%s, want cursor/rules", api.Provider, api.Kind)
	}
	if api.Detail != "applies to **/*.ts" {
		t.Errorf("api.mdc detail = %q, want %q", api.Detail, "applies to **/*.ts")
	}
	if !strings.Contains(api.Body, "named exports") || strings.Contains(api.Body, "globs:") {
		t.Errorf("api.mdc body should be the markdown without its frontmatter:\n%s", api.Body)
	}

	nested, ok := byTitle["components.mdc"]
	if !ok {
		t.Fatal("components.mdc (in a nested folder) not discovered — Cursor documents folders inside .cursor/rules")
	}
	if nested.Detail != "always applied" {
		t.Errorf("components.mdc detail = %q, want %q", nested.Detail, "always applied")
	}

	py, ok := byTitle["py.instructions.md"]
	if !ok {
		t.Fatal("py.instructions.md not discovered")
	}
	if py.Provider != ProviderCopilot {
		t.Errorf("py.instructions.md provider = %s, want copilot", py.Provider)
	}
	if py.Detail != "applies to **/*.py" {
		t.Errorf("py.instructions.md detail = %q, want %q", py.Detail, "applies to **/*.py")
	}

	if _, found := byTitle["notes.md"]; found {
		t.Error("notes.md surfaced from .cursor/rules — Cursor ignores plain .md there, so engram must too")
	}

	// Order within the project: every fixed instruction file, then the rule-dir
	// files, then the index last.
	if pos["api.mdc"] < pos["copilot-instructions.md"] {
		t.Errorf("api.mdc (rank %d) sorted before copilot-instructions.md (rank %d) — rule-dir files come after the fixed files", pos["api.mdc"], pos["copilot-instructions.md"])
	}
	if pos["MEMORY.md"] < pos["api.mdc"] || pos["MEMORY.md"] < pos["py.instructions.md"] {
		t.Error("MEMORY.md must sort after every rule-dir file")
	}
}

// TestDocsSignatureCoversRuleDirs holds the lockstep contract for the rule
// directories, table-driven like its projectRuleFiles sibling — with one extra
// case a fixed path cannot have: a file ADDED to (and then removed from) the
// directory must move the fingerprint, or the poll would never notice a new
// rule file until restart.
func TestDocsSignatureCoversRuleDirs(t *testing.T) {
	for _, rd := range projectRuleDirs {
		t.Run(rd.dir, func(t *testing.T) {
			projectsRoot, realProjDir, _ := buildClaudeTree(t)

			before, err := DocsSignature(projectsRoot, nil)
			if err != nil {
				t.Fatal(err)
			}

			path := filepath.Join(realProjDir, rd.dir, "one"+rd.suffix)
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte("---\ndescription: d\n---\nrule\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			added, err := DocsSignature(projectsRoot, nil)
			if err != nil {
				t.Fatal(err)
			}
			if added == before {
				t.Fatalf("signature unchanged after adding %s — the poll reload will miss new rule files", path)
			}

			// Edit: different content, distinctly newer modtime.
			if err := os.WriteFile(path, []byte("---\ndescription: d2\n---\nrule, revised\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			newer := time.Now().Add(2 * time.Second)
			if err := os.Chtimes(path, newer, newer); err != nil {
				t.Fatal(err)
			}
			edited, err := DocsSignature(projectsRoot, nil)
			if err != nil {
				t.Fatal(err)
			}
			if edited == added {
				t.Errorf("signature unchanged after editing %s", path)
			}

			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			removed, err := DocsSignature(projectsRoot, nil)
			if err != nil {
				t.Fatal(err)
			}
			if removed == edited {
				t.Errorf("signature unchanged after removing %s — a deleted rule file would linger until restart", path)
			}
		})
	}
}
