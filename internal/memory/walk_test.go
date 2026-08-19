package memory

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestEachProjectSkipsNonProjects pins what counts as "a project engram knows
// about": a directory under the projects root carrying a memory/ subdirectory.
// A plain file, and a directory without memory/, must both be skipped — that
// gate used to be written out four times, and this is now the only copy.
func TestEachProjectSkipsNonProjects(t *testing.T) {
	projectsRoot, realProjDir, _ := buildClaudeTree(t)

	// A directory with no memory/ subdir, and a stray file, next to the real ones.
	if err := os.MkdirAll(filepath.Join(projectsRoot, "-Users-me-nomem"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectsRoot, "loose.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	seen := map[string]bool{}
	if err := eachProject(projectsRoot, func(p projectEntry) { seen[p.MemoryDir] = true }); err != nil {
		t.Fatal(err)
	}

	// The two real projects from the fixture: the resolvable one and the ghost.
	if len(seen) != 2 {
		t.Fatalf("eachProject yielded %d projects, want 2: %v", len(seen), seen)
	}
	for _, skipped := range []string{
		filepath.Join(projectsRoot, "-Users-me-nomem", "memory"),
		filepath.Join(projectsRoot, "loose.txt"),
	} {
		if seen[skipped] {
			t.Errorf("eachProject yielded %q, which is not a project", skipped)
		}
	}

	// Every yielded entry must carry a decoded Dir and a Name derived from it.
	if err := eachProject(projectsRoot, func(p projectEntry) {
		if p.Dir == "" || p.Name != filepath.Base(p.Dir) {
			t.Errorf("entry %+v: Name must be filepath.Base(Dir)", p)
		}
	}); err != nil {
		t.Fatal(err)
	}
	_ = realProjDir
}

// TestScansAgreeOnProjectSet is the point of the shared walk: the memory scan
// and the docs scan must resolve the same projects. When each re-implemented
// the walk, one could pick up a project the other missed, and nothing would say
// so — the list and the preview would simply disagree.
func TestScansAgreeOnProjectSet(t *testing.T) {
	projectsRoot, _, _ := buildClaudeTree(t)

	// Seed a real memory file so Discover has something to return per project.
	slugs, err := os.ReadDir(projectsRoot)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range slugs {
		memDir := filepath.Join(projectsRoot, s.Name(), "memory")
		if info, err := os.Stat(memDir); err != nil || !info.IsDir() {
			continue
		}
		body := "---\nname: a\ndescription: d\nmetadata:\n  type: project\n---\n\nbody\n"
		if err := os.WriteFile(filepath.Join(memDir, "a.md"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	var walk []string
	if err := eachProject(projectsRoot, func(p projectEntry) { walk = append(walk, p.MemoryDir) }); err != nil {
		t.Fatal(err)
	}

	mems, err := Discover(projectsRoot)
	if err != nil {
		t.Fatal(err)
	}
	fromMems := map[string]bool{}
	for _, m := range mems {
		fromMems[m.Project.MemoryDir] = true
	}

	docs, err := DiscoverDocs(projectsRoot, nil)
	if err != nil {
		t.Fatal(err)
	}
	fromDocs := map[string]bool{}
	for _, d := range docs {
		if d.MemoryDir != "" { // the global CLAUDE.md has no project
			fromDocs[d.MemoryDir] = true
		}
	}

	for _, memDir := range walk {
		if !fromMems[memDir] {
			t.Errorf("Discover missed project %q that the shared walk found", memDir)
		}
		if !fromDocs[memDir] {
			t.Errorf("DiscoverDocs missed project %q that the shared walk found", memDir)
		}
	}
	if len(fromMems) != len(walk) || len(fromDocs) != len(walk) {
		t.Errorf("project counts disagree: walk=%d Discover=%d DiscoverDocs=%d",
			len(walk), len(fromMems), len(fromDocs))
	}
}

// TestMissingProjectsRootBehaviour pins the four scans' deliberately different
// answers for an absent projects root. Routing them through one helper made it
// tempting to unify these; they are not the same on purpose, and Signature's ""
// in particular is contractual — it matches plan.Signature so the combined
// fingerprint stays stable.
func TestMissingProjectsRootBehaviour(t *testing.T) {
	claudeHome := filepath.Join(t.TempDir(), ".claude") // see buildClaudeTree on why this nests
	if err := os.MkdirAll(claudeHome, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(claudeHome, "CLAUDE.md"), []byte("# global\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(claudeHome, "projects") // never created

	if mems, err := Discover(missing); err != nil || mems != nil {
		t.Errorf("Discover(missing) = %v, %v; want nil, nil", mems, err)
	}

	if sig, err := Signature(missing); err != nil || sig != "" {
		t.Errorf("Signature(missing) = %q, %v; want \"\", nil", sig, err)
	}

	// DiscoverDocs still returns the global CLAUDE.md it already read.
	docs, err := DiscoverDocs(missing, nil)
	if err != nil {
		t.Errorf("DiscoverDocs(missing, nil) error = %v, want nil", err)
	}
	if len(docs) != 1 || docs[0].Scope != "global" {
		t.Errorf("DiscoverDocs(missing, nil) = %+v; want just the global CLAUDE.md", docs)
	}

	// DocsSignature returns a real hash (it covers the global CLAUDE.md), not "".
	sig, err := DocsSignature(missing, nil)
	if err != nil {
		t.Errorf("DocsSignature(missing, nil) error = %v, want nil", err)
	}
	if sig == "" {
		t.Error("DocsSignature(missing, nil) = \"\"; want the hash covering the global CLAUDE.md")
	}
}

// TestScanRootProjectsQualification pins what a scan root actually picks up:
// the root itself and its immediate children, only when they carry an
// instruction file. Without the instruction-file gate every folder under a
// workspace would be listed, including build output and empty directories.
func TestScanRootProjectsQualification(t *testing.T) {
	root := t.TempDir()
	mk := func(rel string, files ...string) string {
		dir := filepath.Join(root, rel)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		for _, f := range files {
			p := filepath.Join(dir, f)
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, []byte("# x\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		return dir
	}

	claudeApp := mk("claude-app", "CLAUDE.md")
	agentsApp := mk("agents-app", "AGENTS.md")
	geminiApp := mk("gemini-app", "GEMINI.md")
	copilotApp := mk("copilot-app", filepath.Join(".github", "copilot-instructions.md"))
	mk("no-rules")                  // qualifies for nothing
	mk("build/output", "CLAUDE.md") // depth 2 — out of reach
	mk(".hidden", "CLAUDE.md")      // dotted dirs are never projects
	deep := filepath.Join(root, "build", "output")

	got := map[string]bool{}
	for _, p := range scanRootProjects([]string{root}, nil) {
		got[p.Dir] = true
		if p.MemoryDir != "" {
			t.Errorf("scanned project %q has MemoryDir %q; scan-root projects have no memory dir", p.Dir, p.MemoryDir)
		}
		if p.Name != filepath.Base(p.Dir) {
			t.Errorf("scanned project %+v: Name must be filepath.Base(Dir)", p)
		}
	}

	for _, want := range []string{claudeApp, agentsApp, geminiApp, copilotApp} {
		if !got[want] {
			t.Errorf("scan missed %q, which carries an instruction file", want)
		}
	}
	for _, unwanted := range []string{
		filepath.Join(root, "no-rules"),
		filepath.Join(root, ".hidden"),
		deep,
	} {
		if got[unwanted] {
			t.Errorf("scan picked up %q, which should not qualify", unwanted)
		}
	}
}

// TestScanRootProjectsDedup covers the overlap case: a directory Claude Code
// already knows about must not appear twice when it also sits under a scan
// root. Claude's copy wins because it is the one carrying a memory dir.
func TestScanRootProjectsDedup(t *testing.T) {
	root := t.TempDir()
	app := filepath.Join(root, "app")
	if err := os.MkdirAll(app, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(app, "AGENTS.md"), []byte("# a\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if got := scanRootProjects([]string{root}, nil); len(got) != 1 || got[0].Dir != app {
		t.Fatalf("unclaimed scan = %+v, want just %q", got, app)
	}
	if got := scanRootProjects([]string{root}, map[string]bool{app: true}); len(got) != 0 {
		t.Errorf("claimed scan = %+v, want none (Claude already owns it)", got)
	}
}

// TestScanRootDocsHaveNoIndexRow is the guard for a path bug the scan makes
// possible: a scan-root project has no memory dir, and filepath.Join("",
// "MEMORY.md") is the *relative* path "MEMORY.md" — which would read whatever
// sits in the process's working directory and show it as that project's index.
func TestScanRootDocsHaveNoIndexRow(t *testing.T) {
	// A MEMORY.md in the working directory: the file the bug would have picked up.
	wd := t.TempDir()
	if err := os.WriteFile(filepath.Join(wd, "MEMORY.md"), []byte("# stray index\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// os.Chdir + restore rather than t.Chdir: this module targets go1.23, and
	// t.Chdir landed in 1.24. No test here runs in parallel, so the process-wide
	// change is contained.
	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(wd); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })

	root := t.TempDir()
	app := filepath.Join(root, "app")
	if err := os.MkdirAll(app, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(app, "GEMINI.md"), []byte("# g\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	missing := filepath.Join(t.TempDir(), ".claude", "projects") // no ~/.claude at all
	docs, err := DiscoverDocs(missing, []string{root})
	if err != nil {
		t.Fatal(err)
	}

	var titles []string
	for _, d := range docs {
		titles = append(titles, d.Title)
		if d.Kind == DocIndex {
			t.Errorf("scan-root project produced an index row %+v — MEMORY.md was read from %q", d, d.Path)
		}
	}
	if len(docs) != 1 || docs[0].Title != "GEMINI.md" || docs[0].ProjectDir != app {
		t.Errorf("scan-root docs = %v (%+v), want just the project's GEMINI.md", titles, docs)
	}
}

// TestScanRootsSurviveMissingClaudeHome is the whole point of the feature: a
// user who has never opened a project in Claude Code still sees their
// instruction files. Scan roots are additive, not a fallback.
func TestScanRootsSurviveMissingClaudeHome(t *testing.T) {
	root := t.TempDir()
	app := filepath.Join(root, "widgets")
	if err := os.MkdirAll(app, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(app, "AGENTS.md"), []byte("# w\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	missing := filepath.Join(t.TempDir(), ".claude", "projects")

	docs, err := DiscoverDocs(missing, []string{root})
	if err != nil {
		t.Fatalf("DiscoverDocs with no ~/.claude = %v, want the scanned project", err)
	}
	if len(docs) != 1 || docs[0].ProjectDir != app {
		t.Fatalf("docs = %+v, want the scanned project's AGENTS.md", docs)
	}

	// The fingerprint must cover it too, or an external edit never reloads.
	before, err := DocsSignature(missing, []string{root})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(app, "AGENTS.md"), []byte("# w, revised\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	newer := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(filepath.Join(app, "AGENTS.md"), newer, newer); err != nil {
		t.Fatal(err)
	}
	after, err := DocsSignature(missing, []string{root})
	if err != nil {
		t.Fatal(err)
	}
	if before == after {
		t.Error("DocsSignature unchanged after editing a scan-root file — the poll reload will miss it")
	}
}

// TestScanRootProjectsRejectsRelative pins that a relative root is ignored
// rather than resolved against the process working directory. Honouring one
// would make the project list depend on where engram was launched from and
// would hand every DocFile a relative Path — the same failure mode as the
// MEMORY.md guard above.
func TestScanRootProjectsRejectsRelative(t *testing.T) {
	root := t.TempDir()
	app := filepath.Join(root, "app")
	if err := os.MkdirAll(app, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(app, "CLAUDE.md"), []byte("# c\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })

	// "." would otherwise resolve to root and find app/.
	for _, rel := range []string{".", "app", "./app", ""} {
		if got := scanRootProjects([]string{rel}, nil); len(got) != 0 {
			t.Errorf("relative root %q yielded %+v, want nothing", rel, got)
		}
	}
	// The absolute form of the same directory still works.
	if got := scanRootProjects([]string{root}, nil); len(got) != 1 || got[0].Dir != app {
		t.Errorf("absolute root = %+v, want %q", got, app)
	}
}

// TestScanRootSymlinkBehaviour documents what the scan actually does with
// symlinks, because the two halves differ and the difference is easy to
// misstate: a symlinked *directory* is not picked up as a project (os.ReadDir
// reports it as a link, not a dir), but a symlinked instruction *file* inside a
// real directory is followed and read, like any other file the user can read.
//
// This is a behaviour record, not a boundary. engram runs as the user and only
// renders the content locally, so following the link is not an escalation —
// but the code comment must not claim the scan cannot leave its root.
func TestScanRootSymlinkBehaviour(t *testing.T) {
	root := t.TempDir()

	// A symlinked directory child: skipped.
	realDir := filepath.Join(t.TempDir(), "linked-proj")
	if err := os.MkdirAll(realDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(realDir, "AGENTS.md"), []byte("# linked\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realDir, filepath.Join(root, "linkdir")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	// A real directory whose AGENTS.md is a symlink to a file outside the root.
	outside := filepath.Join(t.TempDir(), "elsewhere.md")
	if err := os.WriteFile(outside, []byte("# outside\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	linkedFile := filepath.Join(root, "filelink-proj")
	if err := os.MkdirAll(linkedFile, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(linkedFile, "AGENTS.md")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	got := map[string]bool{}
	for _, p := range scanRootProjects([]string{root}, nil) {
		got[filepath.Base(p.Dir)] = true
	}
	if got["linkdir"] {
		t.Error("a symlinked directory was treated as a project; os.ReadDir should report it as a link")
	}
	if !got["filelink-proj"] {
		t.Error("a directory with a symlinked AGENTS.md should still qualify")
	}
}
