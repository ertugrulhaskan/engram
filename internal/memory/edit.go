package memory

import (
	"fmt"
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
	path := filepath.Join(memDir, slug+".md")
	for i := 2; pathExists(path); i++ {
		path = filepath.Join(memDir, fmt.Sprintf("%s-%d.md", slug, i))
	}
	content := fmt.Sprintf("---\nname: %s\ndescription: \nmetadata:\n  type: project\n---\n# %s\n\n", slug, title)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// Delete removes a memory file from disk.
func Delete(path string) error {
	return os.Remove(path)
}
