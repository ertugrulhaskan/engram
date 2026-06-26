package memory

import (
	"crypto/rand"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// EngramMeta is engram's own frontmatter block (the top-level `engram:` key),
// describing how a memory is shared. Claude Code ignores these keys; engram owns
// them entirely and never reads or rewrites Claude's frontmatter fields.
type EngramMeta struct {
	ID      string `yaml:"id,omitempty"`
	Scope   string `yaml:"scope,omitempty"`   // "personal" | "team"
	Project string `yaml:"project,omitempty"` // normalized remote key, or "global"
	Owner   string `yaml:"owner,omitempty"`
}

// ReadEngram extracts the `engram:` block from a memory file's raw contents. ok is
// false when there is no engram block (the file may still have other frontmatter).
func ReadEngram(raw string) (meta EngramMeta, ok bool, err error) {
	fmText, _, has := splitFrontmatter(raw)
	if !has || strings.TrimSpace(fmText) == "" {
		return EngramMeta{}, false, nil
	}
	var doc struct {
		Engram *EngramMeta `yaml:"engram"`
	}
	if err := yaml.Unmarshal([]byte(fmText), &doc); err != nil {
		return EngramMeta{}, false, err
	}
	if doc.Engram == nil {
		return EngramMeta{}, false, nil
	}
	return *doc.Engram, true, nil
}

// WriteEngram returns raw with the `engram:` frontmatter block set to meta. Every
// other frontmatter key, the key order, and the body are preserved; a file with no
// frontmatter gains a block containing only engram's keys. Engram never invents or
// rewrites Claude's fields.
func WriteEngram(raw string, meta EngramMeta) (string, error) {
	fmText, body, has := splitFMVerbatim(raw)

	var root yaml.Node
	if has && strings.TrimSpace(fmText) != "" {
		if err := yaml.Unmarshal([]byte(fmText), &root); err != nil {
			return "", err
		}
	}
	mapping, err := topMapping(&root)
	if err != nil {
		return "", err
	}

	var engramVal yaml.Node
	if err := engramVal.Encode(meta); err != nil {
		return "", err
	}
	setMapKey(mapping, "engram", &engramVal)

	out, err := yaml.Marshal(mapping)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	b.WriteString("---\n")
	b.Write(out) // yaml.Marshal output ends with a newline
	b.WriteString("---\n")
	b.WriteString(body)
	return b.String(), nil
}

// NewID returns a random RFC-4122 v4 UUID for engram.id, built from crypto/rand so
// engram pulls in no external dependency.
func NewID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

// splitFMVerbatim is like splitFrontmatter but returns the body EXACTLY as it
// follows the closing delimiter (no leading-newline trim), so WriteEngram can
// round-trip a file without disturbing its body spacing.
func splitFMVerbatim(raw string) (fm, body string, ok bool) {
	lines := strings.Split(raw, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return "", raw, false
	}
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			return strings.Join(lines[1:i], "\n"), strings.Join(lines[i+1:], "\n"), true
		}
	}
	return "", raw, false
}

// topMapping returns the document's root mapping node, creating a fresh
// document+mapping when there is no frontmatter. It errors when existing
// frontmatter is present but is NOT a mapping (a YAML list or scalar) rather than
// silently dropping it — engram must never discard Claude's frontmatter.
func topMapping(root *yaml.Node) (*yaml.Node, error) {
	if root.Kind == 0 || len(root.Content) == 0 {
		root.Kind = yaml.DocumentNode
		m := &yaml.Node{Kind: yaml.MappingNode}
		root.Content = []*yaml.Node{m}
		return m, nil
	}
	if root.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf("frontmatter is not a YAML mapping")
	}
	return root.Content[0], nil
}

// setMapKey sets key -> val on a mapping node, replacing an existing entry in place
// or appending a new one (so engram's block lands after Claude's keys).
func setMapKey(m *yaml.Node, key string, val *yaml.Node) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			m.Content[i+1] = val
			return
		}
	}
	m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: key}, val)
}
