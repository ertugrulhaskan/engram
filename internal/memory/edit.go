package memory

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

// Slugify converts a title into a kebab-case slug suitable for a filename.
func Slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = slugRe.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

// Create writes a new memory file (with a frontmatter template) into memDir and
// returns its path. The filename is derived from the title and made unique.
func Create(memDir, title string) (string, error) {
	slug := Slugify(title)
	if slug == "" {
		slug = "memory"
	}
	content := fmt.Sprintf("---\nname: %s\ndescription: \nmetadata:\n  type: project\n---\n# %s\n\n", slug, title)
	// Claim the name with O_CREATE|O_EXCL rather than check-then-write. The old
	// "does it exist?" check used os.Stat, which calls a dangling symlink absent,
	// and os.WriteFile then followed that link and created the file wherever it
	// pointed — outside the memory dir. O_EXCL refuses any existing entry at the
	// path, a symlink included, and also closes the race between the check and
	// the write, so the loop just moves on to the next suffix.
	for i := 1; ; i++ {
		name := slug + ".md"
		if i > 1 {
			name = fmt.Sprintf("%s-%d.md", slug, i)
		}
		path := filepath.Join(memDir, name)
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		if _, err := f.WriteString(content); err != nil {
			f.Close()
			os.Remove(path) // don't leave an empty memory behind
			return "", err
		}
		if err := f.Close(); err != nil {
			os.Remove(path)
			return "", err
		}
		return path, nil
	}
}

// Delete removes a memory file from disk.
func Delete(path string) error {
	return os.Remove(path)
}
