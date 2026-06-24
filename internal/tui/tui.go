// Package tui implements engram's Bubble Tea terminal UI. It contains no file
// logic; it consumes parsed memories from the memory package.
package tui

import (
	"github.com/ertugrulhaskan/engram/internal/memory"
)

type focus int

const (
	focusList focus = iota
	focusPreview
)

type mode int

const (
	modeNormal mode = iota
	modeFilter
	modeNew
	modeConfirm
	modePalette
)

// srcKind selects which collection is being browsed.
type srcKind int

const (
	srcMemories srcKind = iota
	srcPlans
	srcFiles // read-only CLAUDE.md / MEMORY.md
)

type groupMode int

const (
	groupProject groupMode = iota
	groupType
)

const (
	badgeWidth  = 11 // width of the widest "[reference]" badge field
	previewPad  = 2  // left margin between the divider and preview content
	maxReadCols = 88 // cap the prose line length on wide terminals for readability
)

// typeCycle is the order the `t` key steps through. "" means "all types".
var typeCycle = []memory.Type{
	"",
	memory.TypeUser,
	memory.TypeFeedback,
	memory.TypeProject,
	memory.TypeReference,
	memory.TypeUnknown,
}
