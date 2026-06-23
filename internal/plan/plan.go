// Package plan discovers Claude Code plan files — the markdown documents written
// in plan mode under ~/.claude/plans/. It contains no UI code.
package plan

import "time"

// Plan is a single plan markdown file. Plans are flat (no project/type/index);
// they carry just a title, body, path, and modification time.
type Plan struct {
	Title    string
	Body     string
	Path     string
	Modified time.Time
}
