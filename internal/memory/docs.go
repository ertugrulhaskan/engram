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

// DocKind classifies a Claude-managed doc surfaced read-only in engram.
type DocKind string

const (
	DocRules DocKind = "rules" // a CLAUDE.md (instructions Claude loads)
	DocIndex DocKind = "index" // a MEMORY.md (the memory index)
)

// DocFile is a CLAUDE.md or MEMORY.md shown read-only in engram's files source.
// These are written/maintained by Claude Code (or via @Claude), not hand-edited
// in engram, so DocFile carries no frontmatter — just enough to display and to
// launch an assistant in the right place.
type DocFile struct {
	Path        string
	Title       string // "CLAUDE.md" or "MEMORY.md"
	Body        string // file contents (markdown)
	Kind        DocKind
	Scope       string // "global" or the project name
	ProjectName string // "" for global
	ProjectDir  string // decoded project dir; "" for global or unresolved
	MemoryDir   string // the project's memory dir; "" for global
	Modified    time.Time
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

// DiscoverDocs returns the read-only Claude docs: the global ~/.claude/CLAUDE.md,
// and per project its CLAUDE.md (only when the project dir resolves on disk) and
// its MEMORY.md. Sorted global-first, then by project, CLAUDE.md before MEMORY.md.
func DiscoverDocs(root string) ([]DocFile, error) {
	claudeHome, projectsRoot, err := claudeLayout(root)
	if err != nil {
		return nil, err
	}

	var docs []DocFile
	read := func(path, title string, kind DocKind, scope, projName, projDir, memDir string) {
		body, err := os.ReadFile(path)
		if err != nil {
			return
		}
		d := DocFile{Path: path, Title: title, Body: string(body), Kind: kind,
			Scope: scope, ProjectName: projName, ProjectDir: projDir, MemoryDir: memDir}
		if info, err := os.Stat(path); err == nil {
			d.Modified = info.ModTime()
		}
		docs = append(docs, d)
	}

	read(filepath.Join(claudeHome, "CLAUDE.md"), "CLAUDE.md", DocRules, "global", "", "", "")

	entries, err := os.ReadDir(projectsRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return docs, nil
		}
		return docs, err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		memDir := filepath.Join(projectsRoot, e.Name(), "memory")
		if info, err := os.Stat(memDir); err != nil || !info.IsDir() {
			continue
		}
		projDir := decodeProjectPath(e.Name())
		projName := filepath.Base(projDir)
		if pathExists(projDir) {
			read(filepath.Join(projDir, "CLAUDE.md"), "CLAUDE.md", DocRules, projName, projName, projDir, memDir)
		}
		read(filepath.Join(memDir, "MEMORY.md"), "MEMORY.md", DocIndex, projName, projName, projDir, memDir)
	}

	sort.SliceStable(docs, func(i, j int) bool {
		gi, gj := docs[i].Scope == "global", docs[j].Scope == "global"
		if gi != gj {
			return gi // global first
		}
		if docs[i].Scope != docs[j].Scope {
			return docs[i].Scope < docs[j].Scope
		}
		return docs[i].Kind == DocRules && docs[j].Kind != DocRules // CLAUDE.md before MEMORY.md
	})
	return docs, nil
}

// DocsSignature fingerprints the same files DiscoverDocs surfaces (path + modtime
// + size), reading no contents — so polling notices external CLAUDE.md/MEMORY.md
// edits. (MEMORY.md is also covered by Signature; the overlap is harmless.)
func DocsSignature(root string) (string, error) {
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

	entries, err := os.ReadDir(projectsRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return strconv.FormatUint(h.Sum64(), 16), nil
		}
		return "", err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		memDir := filepath.Join(projectsRoot, e.Name(), "memory")
		if info, err := os.Stat(memDir); err != nil || !info.IsDir() {
			continue
		}
		if projDir := decodeProjectPath(e.Name()); pathExists(projDir) {
			add(filepath.Join(projDir, "CLAUDE.md"))
		}
		add(filepath.Join(memDir, "MEMORY.md"))
	}
	return strconv.FormatUint(h.Sum64(), 16), nil
}
