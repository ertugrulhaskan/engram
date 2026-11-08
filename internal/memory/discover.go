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
		// A project folder may be a symlink (a moved project kept under its old
		// key); DirEntry.IsDir is false for one, so let it through and let the
		// os.Stat below, which follows links, decide.
		if !e.IsDir() && e.Type()&os.ModeSymlink == 0 {
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
			// os.Stat, not DirEntry.Info: Info is an lstat, so for a symlinked
			// memory it fingerprinted the link itself and an edit to the target
			// never triggered a reload. parseFile already follows the link.
			full := filepath.Join(p.MemoryDir, f.Name())
			info, err := os.Stat(full)
			if err != nil {
				continue
			}
			fmt.Fprintf(h, "%s\x00%d\x00%d\n", full, info.ModTime().UnixNano(), info.Size())
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
// flattens every character outside ASCII letters and digits — "/", ".", "_", a
// space, an accent, a literal "-" — to "-" (see flattenSeparators), so a single
// "-" could originally have been any of them. We recover the real path by walking
// the filesystem — at each directory we look for a real child whose name (put
// through the same flattening) matches a leading run of the
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
		// Match a child by flattening it the way Claude's encoding does, so names
		// like "engram.im", "_clients" or "My Projects" resolve.
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

// flattenSeparators applies Claude Code's project-folder encoding to one name,
// so an on-disk folder can be compared against the encoded tokens. The rule,
// read from Claude Code's own binary (2.1.x), is `path.replace(/[^a-zA-Z0-9]/g,
// "-")`: EVERY character outside ASCII letters and digits becomes "-", not just
// "." and "_" — a space, an accent, "@", "+", "~". JavaScript's replace works on
// UTF-16 code units, so a character outside the BMP (an emoji) becomes two
// dashes. (Above a length cap Claude truncates the name and appends a hash; such
// a key cannot be decoded by anyone and falls back to the slash-joined guess.)
func flattenSeparators(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r > 0xFFFF:
			b.WriteString("--")
		default:
			b.WriteByte('-')
		}
	}
	return b.String()
}

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

// RealPath is p with every symlink resolved, for comparing where two paths
// really are. When p itself doesn't exist (a file not written yet), its
// nearest existing parent is resolved and the rest joined back on, so a
// missing file under a symlinked folder (macOS's /var is /private/var) still
// compares equal to that folder. A dangling symlink on the way is followed to
// its target, not treated as a missing file in place: a link that points out
// of a folder must not count as inside it. Exported for the TUI, which asks
// instead of touching the filesystem itself.
func RealPath(p string) string { return realPath(p, 0) }

// realPathMaxLinks bounds how many dangling links realPath follows, the same
// job ELOOP does for EvalSymlinks.
const realPathMaxLinks = 40

func realPath(p string, links int) string {
	if p == "" {
		return ""
	}
	p = filepath.Clean(p)
	rest := ""
	for dir := p; ; dir = filepath.Dir(dir) {
		if r, err := filepath.EvalSymlinks(dir); err == nil {
			return filepath.Join(r, rest)
		}
		if links < realPathMaxLinks {
			if fi, err := os.Lstat(dir); err == nil && fi.Mode()&os.ModeSymlink != 0 {
				if target, err := os.Readlink(dir); err == nil {
					if !filepath.IsAbs(target) {
						// Relative to where the link really sits: its parent may be
						// a symlink too, and a lexical join would cancel a ".."
						// against the wrong folder.
						target = filepath.Join(realPath(filepath.Dir(dir), links+1), target)
					}
					return realPath(filepath.Join(target, rest), links+1)
				}
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return p // nothing on the way up resolves
		}
		rest = filepath.Join(filepath.Base(dir), rest)
	}
}

// IsDir reports whether p is an existing directory (following symlinks). It is
// exported for the TUI, which asks instead of touching the filesystem itself.
func IsDir(p string) bool {
	if p == "" {
		return false
	}
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}
