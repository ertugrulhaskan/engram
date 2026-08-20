package memory

import (
	"fmt"
	"hash/fnv"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
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
	ProviderCopilot DocProvider = "copilot" // .github/copilot-instructions.md + .github/instructions/*.instructions.md
	ProviderCursor  DocProvider = "cursor"  // .cursor/rules/*.mdc
)

// DocFile is an instruction file shown read-only in engram's files source: a
// project's instruction files (see projectRuleFiles) or a MEMORY.md. These are
// maintained by an assistant (or by hand outside engram), not hand-edited in
// engram, so DocFile carries no frontmatter — just enough to display and to
// launch an assistant in the right place.
type DocFile struct {
	Path        string
	Title       string // the file's own basename, e.g. "CLAUDE.md" or "MEMORY.md"
	Body        string // file contents (markdown); for rule-dir files, the body with its frontmatter split off
	Detail      string // rule-dir files only: one-line scoping note from the frontmatter ("applies to src/**"); "" otherwise
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
// Every entry is a single file at a fixed path; the path-scoped rule
// *directories* live in projectRuleDirs below.
var projectRuleFiles = []ruleFile{
	{"CLAUDE.md", "CLAUDE.md", ProviderClaude},
	{"AGENTS.md", "AGENTS.md", ProviderAgents},
	{"GEMINI.md", "GEMINI.md", ProviderGemini},
	{filepath.Join(".github", "copilot-instructions.md"), "copilot-instructions.md", ProviderCopilot},
}

// ruleDir is a directory of path-scoped rule files engram surfaces read-only:
// every file under dir (recursively — Cursor documents organizing rules in
// folders) whose name ends in suffix. Suffix-strict on purpose: Cursor itself
// ignores a plain .md inside .cursor/rules, so engram matching only .mdc is
// faithful, not a shortcut.
type ruleDir struct {
	dir      string // path relative to the project dir
	suffix   string // filename suffix a rule file must carry
	provider DocProvider
}

// projectRuleDirs lists the rule directories. As with projectRuleFiles,
// DiscoverDocs and DocsSignature both walk this one table (through the shared
// ruleDirFiles), which keeps discovery and the reload fingerprint in lockstep.
//
// Deliberate limits, each with a reason:
//   - Top-level directories only — no monorepo-nested .cursor/rules under
//     arbitrary subdirectories. Finding those means walking the whole repo
//     tree, and DocsSignature re-runs on every 2s poll tick; the same cost
//     argument that fixed scanRoots at depth 1.
//   - No legacy .cursorrules: current Cursor docs document .mdc rules and
//     AGENTS.md (which engram already reads), not the old single file.
//   - No Cursor user rules or Copilot profile instructions: both live inside
//     the app's own settings storage, so there is no file to read — the same
//     gap globalRuleFiles records for Copilot.
//   - Cursor's .cursor/skills and BUGBOT.md are different features, not rules.
var projectRuleDirs = []ruleDir{
	{filepath.Join(".cursor", "rules"), ".mdc", ProviderCursor},
	{filepath.Join(".github", "instructions"), ".instructions.md", ProviderCopilot},
}

// ruleDirFiles enumerates the rule files under one project's ruleDir entry, in
// lexical walk order (deterministic, so list order and the signature are
// stable). A missing or unreadable directory yields nil. Both DiscoverDocs and
// DocsSignature enumerate through here — the shared path is what makes a file
// impossible to surface without also fingerprinting.
func ruleDirFiles(projDir string, rd ruleDir) []string {
	var out []string
	root := filepath.Join(projDir, rd.dir)
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if d != nil && d.IsDir() {
				return fs.SkipDir // unreadable subdir — keep the rest
			}
			return nil // missing dir or unreadable entry — nothing to list
		}
		if !d.IsDir() && strings.HasSuffix(d.Name(), rd.suffix) {
			out = append(out, path)
		}
		return nil
	})
	return out
}

// ruleDetail summarizes a rule file's frontmatter scoping in one line: which
// files the rules bind to (globs / applyTo), "always applied" for
// alwaysApply: true, or the description when no scoping is set. The frontmatter
// boundary comes from splitFrontmatter — the same parse memories use — but the
// keys are read line-wise rather than through yaml.Unmarshal, because real
// .mdc files routinely hold YAML-invalid plain scalars (`globs: **/*.ts`
// starts with an alias character) and a strict parse would blank the one line
// the file exists to state.
func ruleDetail(fm string) string {
	get := func(key string) string {
		for _, ln := range strings.Split(fm, "\n") {
			rest, ok := strings.CutPrefix(strings.TrimSpace(ln), key+":")
			if !ok {
				continue
			}
			v := strings.TrimSpace(rest)
			v = strings.Trim(v, `"'`)
			return v
		}
		return ""
	}
	if v := get("globs"); v != "" {
		return "applies to " + v
	}
	if v := get("applyTo"); v != "" {
		return "applies to " + v
	}
	if strings.EqualFold(get("alwaysApply"), "true") {
		return "always applied"
	}
	return get("description")
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
// projectRuleFiles order, then the rule-dir files in projectRuleDirs order,
// then the memory index last. Ranking off the tables (rather than a
// hand-written switch) means a new entry sorts correctly without a second edit
// here. Ties are deliberate and deterministic: ProviderCopilot ranks by its
// projectRuleFiles row for both its fixed file and its instructions/ files, and
// the files inside one rule dir all share a rank — the stable sort then keeps
// discovery order, which is fixed-file-first, then lexical walk order.
func docRank(d DocFile) int {
	if d.Kind == DocIndex {
		return len(projectRuleFiles) + len(projectRuleDirs)
	}
	for i, rf := range projectRuleFiles {
		if rf.provider == d.Provider {
			return i
		}
	}
	for i, rd := range projectRuleDirs {
		if rd.provider == d.Provider {
			return len(projectRuleFiles) + i
		}
	}
	return len(projectRuleFiles) + len(projectRuleDirs)
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
	read := func(path, title string, kind DocKind, prov DocProvider, scope, projName, projDir, memDir string) *DocFile {
		body, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		d := DocFile{Path: path, Title: title, Body: string(body), Kind: kind, Provider: prov,
			Scope: scope, ProjectName: projName, ProjectDir: projDir, MemoryDir: memDir}
		if info, err := os.Stat(path); err == nil {
			d.Modified = info.ModTime()
		}
		docs = append(docs, d)
		return &docs[len(docs)-1]
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
			// Rule-dir files carry frontmatter (globs / applyTo / description),
			// so their body is split from it — the preview renders the markdown
			// and the scoping surfaces as a one-line Detail instead. The pointer
			// read returns is only valid until the next append, so it is used
			// right here and never held.
			for _, rd := range projectRuleDirs {
				for _, path := range ruleDirFiles(p.Dir, rd) {
					d := read(path, filepath.Base(path), DocRules, rd.provider, p.Name, p.Name, p.Dir, p.MemoryDir)
					if d == nil {
						continue // unreadable — skipped, same as every other doc
					}
					if fmText, body, ok := splitFrontmatter(d.Body); ok {
						d.Body = body
						d.Detail = ruleDetail(fmText)
					}
				}
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
			// Same enumerator as DiscoverDocs, so a rule-dir file cannot be
			// surfaced without being fingerprinted. A removed file drops its
			// line from the hash, so deletions are noticed too.
			for _, rd := range projectRuleDirs {
				for _, path := range ruleDirFiles(p.Dir, rd) {
					add(path)
				}
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
