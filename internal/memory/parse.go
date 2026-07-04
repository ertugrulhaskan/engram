package memory

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// indexLineRe matches a MEMORY.md index line:
//
//   - [Title](file.md) — hook
//
// The separator may be an em-dash, en-dash, or hyphen, and is optional. The
// title group is greedy so a title containing "]" still round-trips (it
// backtracks to the final "](file)"), keeping the file identity reliable.
var indexLineRe = regexp.MustCompile(`^\s*-\s*\[(.+)\]\(([^)]+)\)\s*(?:[—–-]+\s*)?(.*)$`)

// parseFile reads one memory file, supporting both the YAML-frontmatter shape
// and the plain-markdown shape (where metadata comes from the MEMORY.md index).
func parseFile(path string, index map[string]indexEntry) (Memory, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Memory{}, err
	}
	raw := string(data)
	base := filepath.Base(path)

	m := Memory{
		Name: strings.TrimSuffix(base, ".md"),
		Path: path,
		Raw:  raw,
		Type: TypeUnknown,
	}
	if info, err := os.Stat(path); err == nil {
		m.Modified = info.ModTime()
	}

	if e, ok, _ := ReadEngram(raw); ok {
		m.Shared = e // engram sharing block, when present
	}

	fmText, body, hasFM := splitFrontmatter(raw)
	m.Body = body
	if hasFM {
		var fm frontmatter
		if err := yaml.Unmarshal([]byte(fmText), &fm); err == nil {
			if fm.Name != "" {
				m.Name = fm.Name
			}
			if fm.Description != "" {
				m.Description = fm.Description
			}
			if fm.Metadata.Type != "" {
				m.Type = normalizeType(fm.Metadata.Type)
			}
		}
	}

	// Title: first heading, then index, then a titleized slug.
	if h := firstHeading(body); h != "" {
		m.Title = h
	}
	if ie, ok := index[base]; ok {
		if m.Title == "" {
			m.Title = ie.Title
		}
		if m.Description == "" {
			m.Description = ie.Hook
		}
	}
	if m.Title == "" {
		m.Title = titleFromName(m.Name)
	}
	if m.Description == "" {
		m.Description = firstParagraph(body)
	}
	return m, nil
}

// splitFrontmatter separates a leading `---`-delimited YAML block from the body.
// ok is false when there is no frontmatter, in which case body == content.
func splitFrontmatter(content string) (fm, body string, ok bool) {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return "", content, false
	}
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			fm = strings.Join(lines[1:i], "\n")
			body = strings.Join(lines[i+1:], "\n")
			return fm, strings.TrimLeft(body, "\n"), true
		}
	}
	return "", content, false
}

// parseIndex reads a memory dir's MEMORY.md and maps filename -> {title, hook}.
func parseIndex(memoryDir string) map[string]indexEntry {
	out := map[string]indexEntry{}
	data, err := os.ReadFile(filepath.Join(memoryDir, "MEMORY.md"))
	if err != nil {
		return out
	}
	for _, line := range strings.Split(string(data), "\n") {
		mt := indexLineRe.FindStringSubmatch(line)
		if mt == nil {
			continue
		}
		file := strings.TrimSpace(mt[2])
		out[file] = indexEntry{Title: mt[1], Hook: strings.TrimSpace(mt[3])}
	}
	return out
}

func firstHeading(body string) string {
	for _, line := range strings.Split(body, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(t, "# "))
		}
	}
	return ""
}

func firstParagraph(body string) string {
	for _, line := range strings.Split(body, "\n") {
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "#") || strings.HasPrefix(t, "---") {
			continue
		}
		return truncate(t, 100)
	}
	return ""
}

func normalizeType(s string) Type {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "user":
		return TypeUser
	case "feedback":
		return TypeFeedback
	case "project":
		return TypeProject
	case "reference":
		return TypeReference
	default:
		return TypeUnknown
	}
}

func titleFromName(name string) string {
	parts := strings.FieldsFunc(name, func(r rune) bool { return r == '-' || r == '_' })
	for i, p := range parts {
		if p == "" {
			continue
		}
		r := []rune(p) // index by rune so a multibyte leading char isn't split
		parts[i] = strings.ToUpper(string(r[0])) + string(r[1:])
	}
	return strings.Join(parts, " ")
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
