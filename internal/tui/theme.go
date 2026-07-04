package tui

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/ertugrulhaskan/engram/internal/memory"
	"github.com/ertugrulhaskan/engram/internal/team"
)

// Theme is a full color scheme for the UI. Colors are hex strings so they keep
// their identity on truecolor terminals and downsample gracefully elsewhere.
// Backgrounds are applied only to controlled surfaces (the bars and the
// selected row); everything else is foreground color over the terminal's own
// background, so themes read well on any dark terminal.
type Theme struct {
	Name    string
	Glamour string // glamour style name for the preview body

	Accent string // brand, focus, selection chevron, titles
	Fg     string // primary text
	Dim    string // secondary text
	Faint  string // rules, dividers, faint glyphs

	BarBg  string // top/bottom bar background
	SelBg  string // selected row highlight; also preview code chips, inputs, palette header
	Border string // pane divider and rules
	Danger string // destructive actions

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
	return lipgloss.NewStyle().Foreground(lipgloss.Color(c)).Background(lipgloss.Color(t.BarBg))
}

// Semantic status colors for transient footer messages and the index-drift
// warning. They're fixed across themes on purpose: they signal severity
// (danger/cancel), not brand, so the meaning stays constant when the theme
// changes.
const (
	statusDangerBg = "#c0392b" // red
	statusDangerFg = "#ffffff" // white
	statusCancelBg = "#50c878" // emerald
	statusCancelFg = "#3e2723" // dark brown
)

// dangerStyle renders warnings and destructive confirmations (and the index-out-
// of-sync badge): white text on red. cancelStyle renders aborted actions: dark
// brown on emerald.
func dangerStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color(statusDangerFg)).
		Background(lipgloss.Color(statusDangerBg)).
		Bold(true)
}

func cancelStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color(statusCancelFg)).
		Background(lipgloss.Color(statusCancelBg))
}

// Sync-badge colors. Fixed across themes on purpose (like the status colors
// above): they signal a memory's relationship to the team store, not brand. The
// list renders a filled pill (background + contrasting foreground); the glyph is
// kept alongside the word so the state reads without relying on color.
const (
	syncSyncedColor  = "#50c878" // emerald — local matches the team copy
	syncDiffersColor = "#e0af68" // amber — local differs from the team copy
	syncMissingColor = "#c0392b" // red — promoted, but no copy in the store
	syncPillFgDark   = "#0e2118" // dark text, for the green / amber pills
	syncPillFgLight  = "#ffffff" // light text, for the red pill
)

// syncBadge maps a team sync state to its list label, the pill's background and
// foreground colors, and the bare word used (as colored text) in the preview
// meta. The label pairs a width-1-safe glyph with the word so state reads without
// color. StateNone returns empty — personal/unshared memories carry no badge.
func syncBadge(s team.SyncState) (label, bg, fgc, word string) {
	switch s {
	case team.StateSynced:
		return "✓ synced", syncSyncedColor, syncPillFgDark, "synced"
	case team.StateDiffers:
		return "● differs", syncDiffersColor, syncPillFgDark, "differs"
	case team.StateMissing:
		return "! missing", syncMissingColor, syncPillFgLight, "missing"
	default:
		return "", "", "", ""
	}
}

// themes is the switchable set, ordered to match the 1–5 number keys.
var themes = []Theme{
	{
		Name: "Dracula", Glamour: "dracula",
		Accent: "#bd93f9", Fg: "#f8f8f2", Dim: "#6272a4", Faint: "#44475a",
		BarBg: "#21222c", SelBg: "#44475a", Border: "#44475a",
		Danger: "#ff5555",
		TUser:  "#8be9fd", TFeedback: "#ffb86c", TProject: "#50fa7b", TReference: "#bd93f9", TOther: "#6272a4",
		Groups: []string{"#50fa7b", "#8be9fd", "#ff79c6", "#ffb86c", "#bd93f9", "#f1fa8c"},
	},
	{
		Name: "Tokyo Night", Glamour: "tokyo-night",
		Accent: "#7aa2f7", Fg: "#c0caf5", Dim: "#565f89", Faint: "#3b4261",
		BarBg: "#16161e", SelBg: "#283457", Border: "#3b4261",
		Danger: "#f7768e",
		TUser:  "#7dcfff", TFeedback: "#ff9e64", TProject: "#9ece6a", TReference: "#bb9af7", TOther: "#565f89",
		Groups: []string{"#9ece6a", "#7dcfff", "#bb9af7", "#ff9e64", "#7aa2f7", "#e0af68"},
	},
	{
		Name: "Nord", Glamour: "dark",
		Accent: "#88c0d0", Fg: "#e5e9f0", Dim: "#616e88", Faint: "#434c5e",
		BarBg: "#272c36", SelBg: "#3b4252", Border: "#434c5e",
		Danger: "#bf616a",
		TUser:  "#81a1c1", TFeedback: "#d08770", TProject: "#a3be8c", TReference: "#b48ead", TOther: "#616e88",
		Groups: []string{"#a3be8c", "#88c0d0", "#b48ead", "#d08770", "#81a1c1", "#ebcb8b"},
	},
	{
		Name: "Gruvbox", Glamour: "dark",
		Accent: "#fabd2f", Fg: "#ebdbb2", Dim: "#928374", Faint: "#504945",
		BarBg: "#1d2021", SelBg: "#3c3836", Border: "#504945",
		Danger: "#fb4934",
		TUser:  "#83a598", TFeedback: "#fe8019", TProject: "#b8bb26", TReference: "#d3869b", TOther: "#928374",
		Groups: []string{"#b8bb26", "#83a598", "#d3869b", "#fe8019", "#8ec07c", "#fabd2f"},
	},
	{
		Name: "Classic Dark", Glamour: "dark",
		Accent: "#4fa6ed", Fg: "#d4d4d4", Dim: "#808080", Faint: "#3a3a3a",
		BarBg: "#181818", SelBg: "#2a2d2e", Border: "#3a3a3a",
		Danger: "#e05561",
		TUser:  "#569cd6", TFeedback: "#ce9178", TProject: "#6a9955", TReference: "#c586c0", TOther: "#808080",
		Groups: []string{"#6a9955", "#569cd6", "#c586c0", "#ce9178", "#4ec9b0", "#dcdcaa"},
	},
}
