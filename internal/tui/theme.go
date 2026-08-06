package tui

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/ertugrulhaskan/engram/internal/memory"
	"github.com/ertugrulhaskan/engram/internal/team"
)

// Theme is a full color scheme for the UI — one variable set, three value sets.
// Colors are hex strings so they keep their identity on truecolor terminals and
// downsample gracefully elsewhere. Components never reference a raw hex: every
// color in the UI flows through a named token here, which is what lets a light
// theme (Paperback) exist at all.
type Theme struct {
	Name    string // display name, e.g. "Midnight"
	Key     string // persisted config value, e.g. "midnight"
	Glamour string // glamour style name for the preview body

	// Surfaces.
	Bg     string // pane + list background
	Bg2    string // title bar, status bar, code blocks
	BgPane string // preview pane, dialog panels
	Sel    string // selected row; also preview code chips, inputs, palette header
	Edge   string // all 1px borders: pane divider, rules, dialog frames

	// Text.
	Fg     string // primary text
	Dim    string // body copy, secondary text
	Faint  string // chrome, hints, metadata
	Accent string // cursor, active tab, keys, focus border

	// Semantics. Per-theme (not fixed) so they sit correctly on light and dark
	// surfaces alike: OK = synced/global scope, Info = behind/project scope,
	// Warn = ahead/drift/withdraw, Danger = conflict/missing/delete/secret.
	OK     string
	Info   string
	Warn   string
	Danger string
	WarnBg string // drift-banner fill: Warn pre-blended over Bg (terminals have no alpha)

	// Memory type colors.
	TUser, TFeedback, TProject, TReference, TOther string

	// Cycled palette used to color project group headers.
	Groups []string
}

func (t Theme) typeColor(ty memory.Type) string {
	switch ty {
	case memory.TypeUser:
		return t.TUser
	case memory.TypeFeedback:
		return t.TFeedback
	case memory.TypeProject:
		return t.TProject
	case memory.TypeReference:
		return t.TReference
	default:
		return t.TOther
	}
}

func (t Theme) groupColor(i int) string {
	if len(t.Groups) == 0 {
		return t.Accent
	}
	return t.Groups[i%len(t.Groups)]
}

// groupColorer returns a stateful mapper from a group key to a stable, cycling
// header color: each new key (in a contiguous run of equal keys) advances to
// the next color. It's the shared engine behind the grouped memory and plan
// lists — callers must feed keys with equal keys already adjacent.
func (t Theme) groupColorer() func(key string) string {
	idx := -1
	prev := "\x00sentinel"
	return func(key string) string {
		if key != prev {
			idx++
			prev = key
		}
		return t.groupColor(idx)
	}
}

// bar styles bar text: foreground c over the bar background. Used for the top
// and bottom bars so every segment carries the background (lipgloss resets
// would otherwise punch holes in a full-width fill).
func (t Theme) bar(c string) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(c)).Background(lipgloss.Color(t.Bg2))
}

// danger renders warnings and destructive confirmations (and the index-drift
// badge) as a filled block: theme background as text over the Danger fill —
// the state colors are chosen to contrast with Bg on every theme, so Bg is
// always the readable pill text. cancel renders aborted actions the same way
// over the OK fill.
func (t Theme) danger() lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color(t.Bg)).
		Background(lipgloss.Color(t.Danger)).
		Bold(true)
}

func (t Theme) cancel() lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color(t.Bg)).
		Background(lipgloss.Color(t.OK))
}

// syncBadge maps a team sync state to its list label, the pill's background and
// foreground colors, and the bare word used (as colored text) in the preview
// meta. The label pairs a width-1-safe glyph with the word so state reads without
// color. StateNone returns empty — personal/unshared memories carry no badge.
// Semantic mapping per the design tokens: synced→OK, incoming→Info, ahead→Warn,
// conflict/missing→Danger, differs (no anchor)→Faint.
func (t Theme) syncBadge(s team.SyncState) (label, bg, fgc, word string) {
	switch s {
	case team.StateSynced:
		return "✓ synced", t.OK, t.Bg, "synced"
	case team.StateIncoming:
		return "↓ incoming", t.Info, t.Bg, "incoming"
	case team.StateLocalAhead:
		return "↑ ahead", t.Warn, t.Bg, "ahead"
	case team.StateDiverged:
		return "↕ conflict", t.Danger, t.Bg, "conflict"
	case team.StateDiffers:
		return "● differs", t.Faint, t.Bg, "differs"
	case team.StateMissing:
		return "! missing", t.Danger, t.Bg, "missing"
	default:
		return "", "", "", ""
	}
}

// scopeColor maps a scope-chip label to its semantic color ("" when there's no
// chip): global is the team-wide bucket (OK), project a single project's (Info).
func (t Theme) scopeColor(scope string) string {
	switch scope {
	case "global":
		return t.OK
	case "project":
		return t.Info
	default:
		return ""
	}
}

// resolveTheme maps a persisted config value to a theme index. It accepts the
// stable Key ("midnight"), the display Name ("Midnight"), and the names of the
// five retired pre-redesign themes — configs written by older engram versions
// keep working, they just land on Midnight (every retired theme was dark;
// Midnight is the direct heir of the old default). ok is false for empty or
// unrecognized values, so callers choose their own fallback: startup defaults
// to themes[0], while a live settings reload keeps the current theme.
func resolveTheme(v string) (idx int, ok bool) {
	for i, th := range themes {
		if th.Key == v || th.Name == v {
			return i, true
		}
	}
	switch v {
	case "Dracula", "Tokyo Night", "Nord", "Gruvbox", "Classic Dark":
		return 0, true // Midnight
	}
	return 0, false
}

// resolveThemeIdx is resolveTheme with the startup fallback baked in: unknown
// values land on the default theme, matching what older binaries do with the
// new keys.
func resolveThemeIdx(v string) int {
	idx, _ := resolveTheme(v)
	return idx
}

// themes is the switchable set, ordered to match the 1–3 number keys.
// Values are the design-spec token sets (design source: the TUI design spec's
// Dracula Dense / Paper Terminal / Phosphor palettes).
var themes = []Theme{
	{
		Name: "Midnight", Key: "midnight", Glamour: "dracula",
		Bg: "#282a36", Bg2: "#21222c", BgPane: "#2b2d3a", Sel: "#44475a", Edge: "#3a3d4d",
		Fg: "#f8f8f2", Dim: "#c6c8d1", Faint: "#6272a4", Accent: "#bd93f9",
		OK: "#50fa7b", Info: "#8be9fd", Warn: "#ffb86c", Danger: "#ff5555",
		WarnBg: "#3e383b", // Warn @ 10% over Bg
		TUser:  "#8be9fd", TFeedback: "#ffb86c", TProject: "#50fa7b", TReference: "#bd93f9", TOther: "#6272a4",
		Groups: []string{"#50fa7b", "#8be9fd", "#ff79c6", "#ffb86c", "#bd93f9", "#f1fa8c"},
	},
	{
		Name: "Paperback", Key: "paperback", Glamour: "light",
		Bg: "#f5f2ec", Bg2: "#ebe7dd", BgPane: "#fbf9f5", Sel: "#e2ddd0", Edge: "#dcd6c8",
		Fg: "#2a2723", Dim: "#5c574e", Faint: "#928b7d", Accent: "#a8492a",
		OK: "#3f7d4e", Info: "#2f5d8a", Warn: "#a8722a", Danger: "#a83a2a",
		WarnBg: "#eee6db", // Warn @ 9% over Bg
		TUser:  "#2f5d8a", TFeedback: "#a8722a", TProject: "#3f7d4e", TReference: "#6b4a8a", TOther: "#928b7d",
		Groups: []string{"#3f7d4e", "#2f5d8a", "#6b4a8a", "#a8722a", "#a8492a", "#2a6f6a"},
	},
	{
		Name: "CRT", Key: "crt", Glamour: "dark",
		Bg: "#0a0b09", Bg2: "#0f110e", BgPane: "#0d0f0c", Sel: "#1c2419", Edge: "#233020",
		Fg: "#c9f7bf", Dim: "#7fbe78", Faint: "#4d7a4a", Accent: "#ffc05a",
		OK: "#6ef58a", Info: "#7fe4ff", Warn: "#ffc05a", Danger: "#ff6b5a",
		WarnBg: "#1e1910", // Warn @ 8% over Bg
		TUser:  "#7fe4ff", TFeedback: "#ffc05a", TProject: "#6ef58a", TReference: "#d7a0ff", TOther: "#4d7a4a",
		Groups: []string{"#6ef58a", "#7fe4ff", "#d7a0ff", "#ffc05a", "#c9f7bf", "#7fbe78"},
	},
}
