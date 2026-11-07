package memory

import (
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// parseIndexLine parses one MEMORY.md index line:
//
//   - [Title](file.md) — hook
//
// The separator may be an em-dash, en-dash, or hyphen, and is optional.
//
// The link that names the file is the first "](…)" whose target names a memory
// (a bare .md file — see indexTarget), its target read up to the matching ")"
// so balanced parentheses survive ("notes (1).md"). Scanning left to right keeps
// a hook that itself contains a link ("see also [B](b.md)") from stealing the
// line — the old greedy regex read that as b.md, so an upsert of b.md overwrote
// it — while skipping non-memory targets keeps a title that contains a "](" of
// its own ("Fix handlers[i](ctx) call") working, as the greedy regex did. When
// no target names a memory (a URL bullet), the first link is returned.
func parseIndexLine(line string) (title, target, hook string, ok bool) {
	t := strings.TrimLeft(line, " \t")
	if !strings.HasPrefix(t, "-") {
		return "", "", "", false
	}
	t = strings.TrimLeft(t[1:], " \t")
	if !strings.HasPrefix(t, "[") {
		return "", "", "", false
	}
	body := t[1:]
	// First, the title's own closing "]" found by bracket balancing: it handles
	// a title holding brackets ("[x]"), a "](" ("handlers[i](ctx)") or a whole
	// link ("see [B](b.md)"), and a hook holding a link, in one rule. Only when
	// the brackets don't balance does the scan below take over.
	if close := balancedClose(body); close >= 0 && close+1 < len(body) && body[close+1] == '(' {
		for end := close + 2; end < len(body); end++ {
			if body[end] == ')' && indexTarget(strings.TrimSpace(body[close+2:end])) != "" {
				return body[:close], strings.TrimSpace(body[close+2 : end]),
					trimHookSeparator(strings.TrimLeft(body[end+1:], " \t")), true
			}
		}
	}
	found := false
	for i := 0; i+1 < len(body); i++ {
		if body[i] != ']' || body[i+1] != '(' {
			continue
		}
		// Try each ")" after the "(" in turn, so a filename's own parentheses —
		// balanced ("notes (1).md") or not ("notes (draft.md", "a).md") — stay
		// part of the target: the first close that leaves a memory name wins.
		for end := i + 2; end < len(body); end++ {
			if body[end] != ')' {
				continue
			}
			cTarget := strings.TrimSpace(body[i+2 : end])
			cTitle := body[:i]
			cHook := trimHookSeparator(strings.TrimLeft(body[end+1:], " \t"))
			if indexTarget(cTarget) != "" {
				return cTitle, cTarget, cHook, true
			}
			if !found {
				found, title, target, hook = true, cTitle, cTarget, cHook
			}
		}
	}
	return title, target, hook, found
}

// balancedClose returns the index of the "]" that closes a title starting just
// before s (the opening "[" already consumed), or -1 when it never closes.
func balancedClose(s string) int {
	depth := 1
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func trimHookSeparator(rest string) string {
	return strings.TrimSpace(strings.TrimLeft(rest, "—–- \t"))
}

// indexTarget maps a bullet's link target to the memory file it names, or ""
// when it names no memory in this folder: a URL, a link into a subfolder, or a
// non-.md file. Memory folders are flat, so only a bare filename can be one.
// "./a.md" names a.md. The target is taken literally when it is already a bare
// .md name — engram writes bullets with the literal filename, so a file named
// "a#b.md" or "a%20b.md" must round-trip as itself, or every reconcile would
// append another bullet for it — and a "#section" is stripped only when that is
// what keeps it from being one ("a.md#section"). A "" target is not a bullet to
// index drift, so reconcile, which runs after every pull without a confirm,
// leaves such lines alone instead of deleting them as dangling.
func indexTarget(target string) string {
	t := strings.TrimPrefix(strings.TrimSpace(target), "./")
	if bareMemoryName(t) {
		return t
	}
	if i := strings.IndexByte(t, '#'); i >= 0 && bareMemoryName(t[:i]) {
		return t[:i]
	}
	return ""
}

func bareMemoryName(t string) bool {
	return strings.HasSuffix(t, ".md") && len(t) > len(".md") &&
		!strings.Contains(t, "://") && !strings.ContainsAny(t, `/\`) && !strings.Contains(t, "](")
}

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
		title, target, hook, ok := parseIndexLine(line)
		if !ok {
			continue
		}
		if file := indexTarget(target); file != "" {
			out[file] = indexEntry{Title: title, Hook: hook}
		}
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
