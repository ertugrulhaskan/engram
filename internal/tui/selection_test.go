package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/ertugrulhaskan/engram/internal/config"
)

// TestSelectedRowHighlighted guards the list selection styling: exactly one list
// line — the selected (cursor) row — carries a background highlight, and it shows
// the `› ` chevron cue. No other list line paints a background (a stray highlight
// would read as a ghost). The separate bleed guard is TestClampFrameClosesOpenBackground.
func TestSelectedRowHighlighted(t *testing.T) {
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor) // colors must not be stripped, or the check is vacuous
	defer lipgloss.SetColorProfile(old)         // don't leak the global profile to sibling tests

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	base := New(sampleMemories(), samplePlans(), sampleDocs(), config.Config{})

	// Every theme (each has a distinct SelBg) and both groupings — grouping by type
	// adds the right-aligned column, another fill carrier.
	for themeKey := '1'; themeKey <= '5'; themeKey++ {
		for _, group := range []bool{false, true} {
			var cur tea.Model = base
			cur, _ = cur.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
			cur, _ = cur.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{rune(themeKey)}})
			if group {
				cur, _ = cur.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")})
			}
			m := cur.(Model)

			// The list pane in isolation — no top/bottom bars (which carry a BarBg),
			// no divider, no preview.
			pane := m.listPane()
			highlighted, chevronRow := 0, false
			for _, line := range strings.Split(pane, "\n") {
				if seen, _ := scanBackground(line); seen {
					highlighted++
					if strings.Contains(line, "› ") {
						chevronRow = true
					}
				}
			}
			if highlighted != 1 {
				t.Errorf("theme=%c group=%v: %d list lines carry a background highlight, want exactly 1 (the selected row)",
					themeKey, group, highlighted)
			}
			if !chevronRow {
				t.Errorf("theme=%c group=%v: the highlighted row does not show the \"› \" chevron", themeKey, group)
			}
		}
	}
}
