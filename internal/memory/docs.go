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

// ruleFile is one instruction file engram surfaces read-only from a project dir.
type ruleFile struct {
	rel      string // path relative to the project dir
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

// docRank orders docs within one scope: the instruction files in
// projectRuleFiles order, then the memory index last. Ranking off the table
// (rather than a hand-written switch) means a new entry sorts correctly without
// a second edit here.
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

// claudeLayout resolves the ~/.claude home and its projects/ root. If root is
// empty both default under ~/.claude; otherwise root is the projects dir and the
// home is its parent (so a test can point the whole thing at a temp tree).
func claudeLayout(root string) (claudeHome, projectsRoot string, err error) {
	if root != "" {
		return filepath.Dir(root), root, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", err
	}
	claudeHome = filepath.Join(home, ".claude")
	return claudeHome, filepath.Join(claudeHome, "projects"), nil
}

// DiscoverDocs returns the read-only instruction docs: the global
// ~/.claude/CLAUDE.md, and per project every projectRuleFiles entry that exists
// (only when the project dir resolves on disk) plus its MEMORY.md. Sorted
// global-first, then by project, then in projectRuleFiles order with MEMORY.md
// last.
//
// scanRoots adds projects Claude Code has never opened: each root and its
// immediate children are surfaced when they carry an instruction file (see
// scanRootProjects). Those have no memory directory, so they contribute
// instruction files only — no MEMORY.md row.
//
// Only each project's root file is read — the vendors' global equivalents
// (~/.gemini/GEMINI.md and the like) are not per-project and are still not
// covered.
func DiscoverDocs(root string, scanRoots []string) ([]DocFile, error) {
	claudeHome, projectsRoot, err := claudeLayout(root)
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
// them. Both walk projectRuleFiles, which is what keeps them in step: a file
// surfaced there but missed here would reload only on restart.
// (MEMORY.md is also covered by Signature; the overlap is harmless.)
func DocsSignature(root string, scanRoots []string) (string, error) {
	claudeHome, projectsRoot, err := claudeLayout(root)
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
