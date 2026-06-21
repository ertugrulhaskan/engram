// Package memory discovers and parses Claude Code memory files. It contains no
// UI code.
package memory

// Type is the category of a memory, taken from Claude's `metadata.type`.
type Type string

const (
	TypeUser      Type = "user"
	TypeFeedback  Type = "feedback"
	TypeProject   Type = "project"
	TypeReference Type = "reference"
	TypeUnknown   Type = "unknown"
)

// Memory is a single memory file on disk.
type Memory struct {
	Name        string // slug (frontmatter name, or filename without .md)
	Title       string // human-readable title
	Description string // one-line hook
	Type        Type
	Body        string // markdown body, frontmatter stripped
	Raw         string // full original file contents
	Path        string // absolute path on disk
	Project     Project
}

// Project is the Claude Code project a memory belongs to.
type Project struct {
	Name      string // friendly name (basename of decoded dir)
	Dir       string // decoded absolute project dir (best-effort)
	MemoryDir string // .../memory
	Remote    string // git remote URL — populated in v2, empty in v1
}

// frontmatter is the subset of YAML frontmatter engram understands.
type frontmatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Metadata    struct {
		Type string `yaml:"type"`
	} `yaml:"metadata"`
}

// indexEntry is one parsed line of a MEMORY.md index.
type indexEntry struct {
	Title string
	Hook  string
}
