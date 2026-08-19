package memory

import (
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"
)

// DocKind classifies what an instruction doc *is*, independent of which
// assistant wrote it.
type DocKind string

const (
	DocRules DocKind = "rules" // instructions an assistant loads (CLAUDE.md, AGENTS.md)
	DocIndex DocKind = "index" // a MEMORY.md (the memory index)
)

// DocProvider names *whose* doc it is. Kept separate from DocKind because
// AGENTS.md and CLAUDE.md are the same kind (rules) from different ecosystems —
// collapsing the two dimensions is what would force a new DocKind per vendor.
type DocProvider string

const (
	ProviderClaude  DocProvider = "claude"  // CLAUDE.md / MEMORY.md
	ProviderAgents  DocProvider = "agents"  // AGENTS.md, the cross-tool standard
	ProviderGemini  DocProvider = "gemini"  // GEMINI.md, the Gemini CLI's context file
	ProviderCopilot DocProvider = "copilot" // .github/copilot-instructions.md
)

// DocFile is an instruction file shown read-only in engram's files source: a
// project's instruction files (see projectRuleFiles) or a MEMORY.md. These are
// maintained by an assistant (or by hand outside engram), not hand-edited in
// engram, so DocFile carries no frontmatter — just enough to display and to
// launch an assistant in the right place.
type DocFile struct {
	Path        string
	Title       string // the file's own basename, e.g. "CLAUDE.md" or "MEMORY.md"
	Body        string // file contents (markdown)
	Kind        DocKind
	Provider    DocProvider
	Scope       string // "global" or the project name
	ProjectName string // "" for global
	ProjectDir  string // decoded project dir; "" for global or unresolved
	MemoryDir   string // the project's memory dir; "" for global
	Modified    time.Time
}

// ruleFile is one instruction file engram surfaces read-only, at a fixed path
// under some base dir — a project dir for projectRuleFiles, the user's home dir
// for globalRuleFiles.
type ruleFile struct {
	rel      string // path relative to that base dir
	title    string // display name (the file's own basename)
	provider DocProvider
}

// projectRuleFiles lists those files in display order — Claude's own rules
// first, then the cross-tool AGENTS.md, then the other vendors'. DiscoverDocs
// and DocsSignature both walk this one table, which is what keeps them in
// lockstep: a file surfaced by the walk but missed by the fingerprint would
// render fine and then never refresh after an external edit.
//
// Every entry is a single file at a fixed path. The path-scoped variants —
// Copilot's .github/instructions/*.instructions.md and Cursor's
// .cursor/rules/*.mdc — are directories of frontmatter-bearing files, which
// need more than DocFile's flat shape, so they are deliberately absent.
var projectRuleFiles = []ruleFile{
	{"CLAUDE.md", "CLAUDE.md", ProviderClaude},
	{"AGENTS.md", "AGENTS.md", ProviderAgents},
	{"GEMINI.md", "GEMINI.md", ProviderGemini},
	{filepath.Join(".github", "copilot-instructions.md"), "copilot-instructions.md", ProviderCopilot},
}

// globalRuleFiles lists the vendors' *global* instruction files — the ones that
// apply to every project and therefore live in the user's home dir, outside the
// ~/.claude tree the project walk is anchored to. Claude's own
// ~/.claude/CLAUDE.md is read separately because it sits under claudeHome
// rather than home, so it is deliberately absent here.
//
// As with projectRuleFiles, DiscoverDocs and DocsSignature both walk this one
// table, which is what keeps discovery and the reload fingerprint in lockstep.
//
// Absent on purpose:
//   - Copilot — VS Code keeps user-level instructions in profile settings, not
//     a markdown file at a fixed path in the home dir, so there is nothing to
//     read.
//   - Codex's CODEX_HOME relocation and its AGENTS.override.md (which wins over
//     AGENTS.md when both exist) — engram surfaces the default file only.
var globalRuleFiles = []ruleFile{
	{filepath.Join(".codex", "AGENTS.md"), "AGENTS.md", ProviderAgents},
	{filepath.Join(".gemini", "GEMINI.md"), "GEMINI.md", ProviderGemini},
}

// docRank orders docs within one scope: the instruction files in
// projectRuleFiles order, then the memory index last. Ranking off the table
// (rather than a hand-written switch) means a new entry sorts correctly without
// a second edit here. It ranks the global scope too — globalRuleFiles reuses
// the same providers, so the one table orders both scopes consistently.
func docRank(d DocFile) int {
	if d.Kind == DocIndex {
		return len(projectRuleFiles)
	}
	for i, rf := range projectRuleFiles {
		if rf.provider == d.Provider {
			return i
		}
	}
	return len(projectRuleFiles)
}

// claudeLayout resolves the user's home dir, the ~/.claude home under it, and
// the projects/ root under that. If root is empty all three derive from the real
// home dir; otherwise root is the projects dir, ~/.claude is its parent and home
// is the parent of that — so a test can point the whole tree, vendor dirs
// included, at a temp dir.
//
// That last hop is why a fixture must nest its projects root as
// <tmp>/.claude/projects. A shallower fixture would resolve home to whatever
// directory happens to hold the temp tree, and the globalRuleFiles lookups would
// read the developer's real ~/.gemini/GEMINI.md mid-test.
func claudeLayout(root string) (home, claudeHome, projectsRoot string, err error) {
	if root != "" {
		claudeHome = filepath.Dir(root)
		return filepath.Dir(claudeHome), claudeHome, root, nil
	}
	home, err = os.UserHomeDir()
	if err != nil {
		return "", "", "", err
	}
	claudeHome = filepath.Join(home, ".claude")
	return home, claudeHome, filepath.Join(claudeHome, "projects"), nil
}

// DiscoverDocs returns the read-only instruction docs: the global
// ~/.claude/CLAUDE.md plus every globalRuleFiles entry that exists, and per
// project every projectRuleFiles entry that exists (only when the project dir
// resolves on disk) plus its MEMORY.md. Sorted global-first, then by project,
// then in projectRuleFiles order with MEMORY.md last.
//
// scanRoots adds projects Claude Code has never opened: each root and its
// immediate children are surfaced when they carry an instruction file (see
// scanRootProjects). Those have no memory directory, so they contribute
// instruction files only — no MEMORY.md row.
//
// A global doc belongs to no project, so it carries empty ProjectName,
// ProjectDir and MemoryDir — the same shape ~/.claude/CLAUDE.md has always had.
// The TUI already reads that as "no project to open" (assistantContext returns
// claudeHome() when both dirs are empty), so @Claude launches in ~/.claude
// rather than pointing at someone else's repo.
func DiscoverDocs(root string, scanRoots []string) ([]DocFile, error) {
	home, claudeHome, projectsRoot, err := claudeLayout(root)
	if err != nil {
		return nil, err
	}

	var docs []DocFile
	read := func(path, title string, kind DocKind, prov DocProvider, scope, projName, projDir, memDir string) {
		body, err := os.ReadFile(path)
		if err != nil {
			return
		}
		d := DocFile{Path: path, Title: title, Body: string(body), Kind: kind, Provider: prov,
			Scope: scope, ProjectName: projName, ProjectDir: projDir, MemoryDir: memDir}
		if info, err := os.Stat(path); err == nil {
			d.Modified = info.ModTime()
		}
		docs = append(docs, d)
	}

	read(filepath.Join(claudeHome, "CLAUDE.md"), "CLAUDE.md", DocRules, ProviderClaude, "global", "", "", "")
	for _, rf := range globalRuleFiles {
		read(filepath.Join(home, rf.rel), rf.title, DocRules, rf.provider, "global", "", "", "")
	}

	err = allProjects(projectsRoot, scanRoots, func(p projectEntry) {
		if pathExists(p.Dir) {
			for _, rf := range projectRuleFiles {
				read(filepath.Join(p.Dir, rf.rel), rf.title, DocRules, rf.provider, p.Name, p.Name, p.Dir, p.MemoryDir)
			}
		}
		// A scan-root project has no memory dir. Guard it: filepath.Join("",
		// "MEMORY.md") is the *relative* path "MEMORY.md", which would read
		// whatever happens to sit in the working directory.
		if p.MemoryDir != "" {
			read(filepath.Join(p.MemoryDir, "MEMORY.md"), "MEMORY.md", DocIndex, ProviderClaude, p.Name, p.Name, p.Dir, p.MemoryDir)
		}
	})
	if err != nil && !os.IsNotExist(err) {
		return docs, err // deliberately unsorted: same as before the walk was shared
	}

	sort.SliceStable(docs, func(i, j int) bool {
		gi, gj := docs[i].Scope == "global", docs[j].Scope == "global"
		if gi != gj {
			return gi // global first
		}
		if docs[i].Scope != docs[j].Scope {
			return docs[i].Scope < docs[j].Scope
		}
		return docRank(docs[i]) < docRank(docs[j])
	})
	return docs, nil
}

// DocsSignature fingerprints the same files DiscoverDocs surfaces (path + modtime
// + size), reading no contents — so polling notices an external edit to any of
// them. Both walk projectRuleFiles and globalRuleFiles, which is what keeps them
// in step: a file surfaced there but missed here would reload only on restart.
// (MEMORY.md is also covered by Signature; the overlap is harmless.)
func DocsSignature(root string, scanRoots []string) (string, error) {
	home, claudeHome, projectsRoot, err := claudeLayout(root)
	if err != nil {
		return "", err
	}
	h := fnv.New64a()
	add := func(path string) {
		if info, err := os.Stat(path); err == nil {
			fmt.Fprintf(h, "%s\x00%d\x00%d\n", path, info.ModTime().UnixNano(), info.Size())
		}
	}
	add(filepath.Join(claudeHome, "CLAUDE.md"))
	for _, rf := range globalRuleFiles {
		add(filepath.Join(home, rf.rel))
	}

	err = allProjects(projectsRoot, scanRoots, func(p projectEntry) {
		if pathExists(p.Dir) {
			for _, rf := range projectRuleFiles {
				add(filepath.Join(p.Dir, rf.rel))
			}
		}
		if p.MemoryDir != "" { // see the guard in DiscoverDocs
			add(filepath.Join(p.MemoryDir, "MEMORY.md"))
		}
	})
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	return strconv.FormatUint(h.Sum64(), 16), nil
}
