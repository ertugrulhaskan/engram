package memory

import (
	"crypto/rand"
	"crypto/sha256"
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
	// SyncedHash is the ContentDigest of the shared content at the last sync
	// (promote or pull) — the common base engram compares against to tell a clean
	// fast-forward from a real conflict. Empty on personal memories and on copies
	// last synced by a pre-anchor engram; callers must treat an absent anchor as
	// "unknown" and fall back to a direction-less compare.
	SyncedHash string `yaml:"syncedHash,omitempty"`
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
	return reserialize(mapping, body)
}

// reserialize renders a frontmatter mapping node plus a verbatim body back into a
// memory file with the standard `---` delimiters. Shared by WriteEngram and
// stripEngramBlock so the digest input and the file written to disk stay
// byte-identical — if the framing ever changes, both move together.
func reserialize(mapping *yaml.Node, body string) (string, error) {
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

// ContentDigest returns a stable short hex hash of a memory's shared content —
// its body plus Claude's frontmatter — with engram's own `engram:` block excluded
// so bookkeeping (the sync anchor, owner, scope, id) never perturbs it, and line
// endings normalized. Two files that carry the same body and Claude frontmatter
// digest equal regardless of engram metadata or CRLF/LF. It underpins the sync
// anchor: a copy's stored SyncedHash is the digest of the content it was last
// synced to, so a later digest mismatch means that copy's content has moved.
func ContentDigest(raw string) (string, error) {
	canon, err := stripEngramBlock(raw)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(strings.ReplaceAll(canon, "\r\n", "\n")))
	// 8 bytes (64 bits) is ample for a personal/team memory store — accidental
	// collisions are astronomically unlikely, and a mismatch only ever degrades to a
	// conservative conflict, never a silent overwrite.
	return fmt.Sprintf("%x", sum[:8]), nil
}

// ShareContent returns a memory's shared content — Claude's frontmatter and body
// with engram's own block removed and the frontmatter canonicalized. It is exactly
// what ContentDigest hashes and what the conflict editor presents, so a resolution
// saved from it re-digests consistently against the store copy.
func ShareContent(raw string) (string, error) { return stripEngramBlock(raw) }

// stripEngramBlock returns raw with the `engram:` frontmatter key removed and the
// remaining frontmatter re-serialized canonically; the body is preserved verbatim.
// A file with no frontmatter (or whose frontmatter held only the engram block)
// collapses to its body. Used only to compute ContentDigest — never written to disk.
func stripEngramBlock(raw string) (string, error) {
	fmText, body, has := splitFMVerbatim(raw)
	if !has || strings.TrimSpace(fmText) == "" {
		return raw, nil
	}
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(fmText), &root); err != nil {
		return "", err
	}
	mapping, err := topMapping(&root)
	if err != nil {
		return "", err
	}
	deleteMapKey(mapping, "engram")
	if len(mapping.Content) == 0 {
		return body, nil // frontmatter held only the engram block
	}
	return reserialize(mapping, body)
}

// deleteMapKey removes key (and its value) from a mapping node, in place.
func deleteMapKey(m *yaml.Node, key string) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			m.Content = append(m.Content[:i], m.Content[i+2:]...)
			return
		}
	}
}

// SetBody returns raw with its markdown body replaced by newBody, preserving the
// frontmatter block (delimiters and every key) verbatim. A file with no
// frontmatter becomes newBody alone.
func SetBody(raw, newBody string) string {
	fm, _, has := splitFMVerbatim(raw)
	if !has {
		return newBody
	}
	return "---\n" + fm + "\n---\n" + newBody
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
