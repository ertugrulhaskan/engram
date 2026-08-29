// Package plan discovers Claude Code plan files — the markdown documents written
// in plan mode under ~/.claude/plans/. It contains no UI code.
package plan

import (
	"time"

	"github.com/ertugrulhaskan/engram/internal/source"
)

// Caps declares what the TUI may offer the user on plans: view and delete
// only. Plans are plan-mode output engram has no business editing, but
// deleting a stale one is ordinary housekeeping (ENGR-12).
var Caps = source.Caps{Delete: true}

// Plan is a single plan markdown file. Plans are flat (no project/type/index);
// they carry just a title, body, path, and modification time.
type Plan struct {
	Title    string
	Body     string
	Path     string
	Modified time.Time
}
