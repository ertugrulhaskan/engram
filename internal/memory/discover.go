package memory

import (
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// projectEntry is one project engram knows about, as found by the projects walk:
// the memory directory that identifies it, plus the working directory its folder
// key decodes to (which may no longer exist on disk).
type projectEntry struct {
	MemoryDir string // <projectsRoot>/<encoded>/memory — always a real directory
	Dir       string // decoded working directory; may not exist
	Name      string // display name, filepath.Base(Dir)
}

// eachProject walks projectsRoot and calls fn once per project that has a
// memory/ directory — the single definition of "a project engram knows about".
// Discover, Signature, DiscoverDocs and DocsSignature all go through here, so
// they cannot disagree about which projects exist; they used to re-implement
// this same walk four times, which is what made that drift possible.
//
// The ReadDir error is returned unwrapped rather than swallowed, because the
// four callers deliberately differ on a missing root: two return an empty
// result, one returns the docs it already has, and Signature returns "" rather
// than the hash of nothing. Each keeps its own os.IsNotExist branch.
func eachProject(projectsRoot string, fn func(projectEntry)) error {
	entries, err := os.ReadDir(projectsRoot)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		memDir := filepath.Join(projectsRoot, e.Name(), "memory")
		if info, err := os.Stat(memDir); err != nil || !info.IsDir() {
			continue
		}
		dir := decodeProjectPath(e.Name())
		fn(projectEntry{MemoryDir: memDir, Dir: dir, Name: filepath.Base(dir)})
	}
	return nil
}

// Discover walks every Claude project under root and returns all memories found.
// If root is empty it defaults to ~/.claude/projects.
func Discover(root string) ([]Memory, error) {
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		root = filepath.Join(home, ".claude", "projects")
	}

	var mems []Memory
	err := eachProject(root, func(p projectEntry) {
		proj := Project{Dir: p.Dir, Name: p.Name, MemoryDir: p.MemoryDir}
		index := parseIndex(p.MemoryDir)

		files, err := os.ReadDir(p.MemoryDir)
		if err != nil {
			return
		}
		for _, f := range files {
			if f.IsDir() {
				continue
			}
			name := f.Name()
			if name == "MEMORY.md" || !strings.HasSuffix(name, ".md") {
				continue
			}
			m, err := parseFile(filepath.Join(p.MemoryDir, name), index)
			if err != nil {
				continue
			}
			m.Project = proj
			mems = append(mems, m)
		}
	})
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // no projects dir yet → no memories (not an error)
		}
		return nil, err
	}

	sort.Slice(mems, func(i, j int) bool {
		if mems[i].Project.Name != mems[j].Project.Name {
			return mems[i].Project.Name < mems[j].Project.Name
		}
		return mems[i].Title < mems[j].Title
	})
	return mems, nil
}

// Signature returns a cheap fingerprint of every memory file under root (each
// file's path + modtime + size, including MEMORY.md so index edits count too).
// It changes whenever a memory is added, removed, or edited, and reads no file
// contents — used to poll for external changes. It mirrors Discover's walk so
// the two never disagree about which files exist. If root is empty it defaults
// to ~/.claude/projects.
func Signature(root string) (string, error) {
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		root = filepath.Join(home, ".claude", "projects")
	}

	h := fnv.New64a()
	err := eachProject(root, func(p projectEntry) {
		files, err := os.ReadDir(p.MemoryDir)
		if err != nil {
			return
		}
		for _, f := range files {
			if f.IsDir() || !strings.HasSuffix(f.Name(), ".md") {
				continue
			}
			info, err := f.Info()
			if err != nil {
				continue
			}
			fmt.Fprintf(h, "%s\x00%d\x00%d\n",
				filepath.Join(p.MemoryDir, f.Name()), info.ModTime().UnixNano(), info.Size())
		}
	})
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil // no projects dir → empty fingerprint, matches plan.Signature
		}
		return "", err
	}
	return strconv.FormatUint(h.Sum64(), 16), nil
}

// decodeProjectPath turns Claude's encoded project folder name (e.g.
// "-Users-me-code-app") back into a real path. The encoding is lossy: Claude
// flattens "/", ".", "_" and any literal "-" all to "-", so a single "-" could
// originally have been any of them. We recover the real path by walking the
// filesystem — at each directory we look for a real child whose name (with its
// own dots and underscores flattened to dashes) matches a leading run of the
// remaining tokens. This reconstructs multi-separator names like "engram.im", a
// domain-style "app.engram.im", or an underscored "_clients" that a
// token-at-a-time probe can't. The walk is still
// best-effort and inherently ambiguous: if the real project folder is gone but a
// flattened-equivalent sibling exists (e.g. "/Users/me.app" when "/Users/me/app"
// was deleted) it resolves to the sibling. When nothing resolves we fall back to
// a best-effort slash-joined path.
func decodeProjectPath(encoded string) string {
	if !strings.HasPrefix(encoded, "-") {
		return encoded
	}
	tokens := strings.Split(encoded[1:], "-")
	if resolved, ok := resolveTokens("/", tokens); ok {
		return resolved
	}
	return filepath.Join(append([]string{"/"}, tokens...)...)
}

// resolveTokens reconstructs a real path under base by consuming every token. A
// single filesystem child can absorb several tokens at once when its name held a
// "-" or "." the encoding flattened away. We try the match that consumes the
// fewest tokens first — treating the next "-" as a "/" — to mirror the historical
// "slash first" preference, and backtrack when a branch dead-ends.
func resolveTokens(base string, tokens []string) (string, bool) {
	if len(tokens) == 0 {
		return base, true
	}
	entries, err := os.ReadDir(base)
	if err != nil {
		return "", false
	}
	type match struct {
		name string
		n    int // tokens this child consumes
	}
	var matches []match
	for _, e := range entries {
		// Match a child by flattening the same characters Claude's encoding does
		// (".", "_", "-" → "-") so names like "engram.im" or "_clients" resolve.
		parts := strings.Split(flattenSeparators(e.Name()), "-")
		if tokensHavePrefix(tokens, parts) {
			matches = append(matches, match{e.Name(), len(parts)})
		}
	}
	sort.SliceStable(matches, func(i, j int) bool { return matches[i].n < matches[j].n })
	for _, m := range matches {
		if resolved, ok := resolveTokens(filepath.Join(base, m.name), tokens[m.n:]); ok {
			return resolved, true
		}
	}
	return "", false
}

// flattenSeparators maps the characters Claude's project-folder encoding collapses
// to "-" (".", "_", and "-" itself already is one) so an on-disk folder name can be
// compared against the encoded tokens.
var separatorFlattener = strings.NewReplacer(".", "-", "_", "-")

func flattenSeparators(name string) string { return separatorFlattener.Replace(name) }

// tokensHavePrefix reports whether prefix matches the leading elements of tokens.
func tokensHavePrefix(tokens, prefix []string) bool {
	if len(prefix) > len(tokens) {
		return false
	}
	for i, p := range prefix {
		if tokens[i] != p {
			return false
		}
	}
	return true
}

func pathExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// expandHome resolves a leading "~" against the user's home directory. Config
// values are hand-written, so "~/code" is the form people actually type.
func expandHome(p string) string {
	if p != "~" && !strings.HasPrefix(p, "~/") {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	if p == "~" {
		return home
	}
	return filepath.Join(home, p[2:])
}

// hasRuleFile reports whether dir carries at least one of the instruction files
// engram surfaces. This is what qualifies a scanned directory as a project:
// without it every folder under a scan root would be listed, including empty
// ones and build output. The rule dirs are checked only after the fixed paths
// all miss, so the common case stays a handful of stats; a non-empty
// .cursor/rules alone qualifies a project, since /files would list its files.
func hasRuleFile(dir string) bool {
	for _, rf := range projectRuleFiles {
		if pathExists(filepath.Join(dir, rf.rel)) {
			return true
		}
	}
	for _, rd := range projectRuleDirs {
		if len(ruleDirFiles(dir, rd)) > 0 {
			return true
		}
	}
	return false
}

// scanRootProjects finds projects under the configured scan roots: each root
// itself, plus its immediate children. Directories already claimed by Claude's
// own walk are skipped, so a project in both places appears once — Claude's copy
// wins, because it is the one carrying a memory directory.
//
// Depth is deliberately 1. DocsSignature re-runs this on every poll tick, so a
// recursive walk would re-read a whole workspace tree several times a minute;
// one ReadDir per root plus a few stats per candidate stays negligible. Dotted
// directories are skipped — .git and friends are never projects.
//
// Two things are deliberately not supported, both because the alternative is
// surprising rather than useful:
//
//   - A root must be absolute (after "~" expansion). A relative root would
//     resolve against the process working directory, so the project list would
//     change depending on where engram was launched from, and every resulting
//     DocFile.Path would be relative. Non-absolute roots are skipped.
//   - Symlinked *directories* are not descended into. os.ReadDir reports a
//     symlink as a link rather than a directory, so a linked project directory
//     is not picked up as a candidate. Note this does not extend to the
//     instruction files themselves: a project whose AGENTS.md is a symlink is
//     qualified and that file is read through the link, like any other file the
//     user can already read. Containment is not a security boundary here —
//     engram runs as the user and only renders the content locally — but do not
//     read this as "the scan cannot leave the root", because it can.
//
// The returned entries have no MemoryDir: a directory Claude Code has never
// opened has no memory folder, so these contribute instruction files only.
func scanRootProjects(roots []string, claimed map[string]bool) []projectEntry {
	var out []projectEntry
	seen := map[string]bool{}
	for _, root := range roots {
		root = strings.TrimSpace(expandHome(root))
		if root == "" || !filepath.IsAbs(root) {
			continue
		}
		candidates := []string{filepath.Clean(root)}
		if entries, err := os.ReadDir(root); err == nil {
			for _, e := range entries {
				if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
					candidates = append(candidates, filepath.Join(root, e.Name()))
				}
			}
		}
		for _, dir := range candidates {
			if claimed[dir] || seen[dir] || !hasRuleFile(dir) {
				continue
			}
			seen[dir] = true
			out = append(out, projectEntry{Dir: dir, Name: filepath.Base(dir)})
		}
	}
	// Stable order regardless of ReadDir order or how the roots were listed.
	sort.Slice(out, func(i, j int) bool { return out[i].Dir < out[j].Dir })
	return out
}

// allProjects enumerates every project engram surfaces: the ones Claude Code
// knows about under projectsRoot, then any additional ones found under
// scanRoots. Both docs scans go through this, which is what keeps them in
// lockstep once two different discovery mechanisms feed the same list.
//
// Scan roots are additive and independent: they are walked even when
// projectsRoot is missing or unreadable, so a user with no ~/.claude still sees
// their configured projects. The projectsRoot error is returned for the caller
// to interpret, since the callers deliberately differ on it.
func allProjects(projectsRoot string, scanRoots []string, fn func(projectEntry)) error {
	claimed := map[string]bool{}
	err := eachProject(projectsRoot, func(p projectEntry) {
		claimed[p.Dir] = true
		fn(p)
	})
	for _, p := range scanRootProjects(scanRoots, claimed) {
		fn(p)
	}
	return err
}
