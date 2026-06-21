package memory

import (
	"os"
	"path/filepath"
	"sort"
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

// decodeProjectPath turns Claude's encoded project folder name (e.g.
// "-Users-me-code-app") back into a real path. Decoding is ambiguous because
// path segments may contain "-", so we probe the filesystem: prefer "/", fall
// back to "-". When nothing resolves we return a best-effort slash-joined path.
func decodeProjectPath(encoded string) string {
	if !strings.HasPrefix(encoded, "-") {
		return encoded
	}
	tokens := strings.Split(encoded[1:], "-")
	path := "/"
	for i, tok := range tokens {
		if i == 0 {
			path += tok
			continue
		}
		if withSlash := filepath.Join(path, tok); pathExists(withSlash) {
			path = withSlash
			continue
		}
		if withDash := path + "-" + tok; pathExists(withDash) {
			path = withDash
			continue
		}
		path = filepath.Join(path, tok)
	}
	return path
}

func pathExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
