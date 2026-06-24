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

	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // no projects dir yet → no memories (not an error)
		}
		return nil, err
	}

	var mems []Memory
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		memDir := filepath.Join(root, e.Name(), "memory")
		if info, err := os.Stat(memDir); err != nil || !info.IsDir() {
			continue
		}

		proj := Project{Dir: decodeProjectPath(e.Name()), MemoryDir: memDir}
		proj.Name = filepath.Base(proj.Dir)

		index := parseIndex(memDir)

		files, err := os.ReadDir(memDir)
		if err != nil {
			continue
		}
		for _, f := range files {
			if f.IsDir() {
				continue
			}
			name := f.Name()
			if name == "MEMORY.md" || !strings.HasSuffix(name, ".md") {
				continue
			}
			m, err := parseFile(filepath.Join(memDir, name), index)
			if err != nil {
				continue
			}
			m.Project = proj
			mems = append(mems, m)
		}
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

	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil // no projects dir → empty fingerprint, matches plan.Signature
		}
		return "", err
	}

	h := fnv.New64a()
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		memDir := filepath.Join(root, e.Name(), "memory")
		if info, err := os.Stat(memDir); err != nil || !info.IsDir() {
			continue
		}
		files, err := os.ReadDir(memDir)
		if err != nil {
			continue
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
				filepath.Join(memDir, f.Name()), info.ModTime().UnixNano(), info.Size())
		}
	}
	return strconv.FormatUint(h.Sum64(), 16), nil
}

// decodeProjectPath turns Claude's encoded project folder name (e.g.
// "-Users-me-code-app") back into a real path. The encoding is lossy: Claude
// flattens "/", "." and any literal "-" all to "-", so a single "-" could
// originally have been any of the three. We recover the real path by walking the
// filesystem — at each directory we look for a real child whose name (with its
// own dots flattened to dashes) matches a leading run of the remaining tokens.
// This reconstructs multi-separator names like "engram.im" or a domain-style
// "app.engram.im" that a token-at-a-time probe can't. The walk is still
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
		parts := strings.Split(strings.ReplaceAll(e.Name(), ".", "-"), "-")
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
