package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/ertugrulhaskan/engram/internal/config"
)

// selBgParams is the truecolor SGR parameter run that sets th's selected-row
// background — what the highlight assertions grep for. Derived through termenv
// (the same path lipgloss renders with) rather than recomputed from the hex:
// termenv truncates the float round-trip (uint8(x/255*255)), so hand-computed
// integer params can be off by one on some channels.
func selBgParams(th Theme) string {
	return termenv.TrueColor.Color(th.Sel).Sequence(true)
}

// TestSelectedRowHighlighted guards the list selection styling under the
// full-background paint pass: every list line carries the theme surface, but
// exactly one — the selected (cursor) row — carries the Sel highlight on top,
// and it shows the `› ` chevron cue. A second Sel row would read as a ghost
// selection. The bleed guard is TestClampFrameClosesOpenBackground.
func TestSelectedRowHighlighted(t *testing.T) {
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor) // colors must not be stripped, or the check is vacuous
	defer lipgloss.SetColorProfile(old)         // don't leak the global profile to sibling tests

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	base := New(sampleMemories(), samplePlans(), sampleDocs(), config.Config{})

	// Every theme (each has a distinct Sel) and both groupings — grouping by type
	// adds the right-aligned column, another fill carrier.
	for themeKey := '1'; themeKey <= '3'; themeKey++ {
		sel := selBgParams(themes[themeKey-'1'])
		for _, group := range []bool{false, true} {
			var cur tea.Model = base
			cur, _ = cur.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
			cur, _ = cur.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{rune(themeKey)}})
			if group {
				cur, _ = cur.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")})
			}
			m := cur.(Model)

			// The list pane in isolation — no top/bottom bars (which carry a Bg2),
			// no divider, no preview.
			pane := m.listPane()
			selRows, chevronRow, unpainted := 0, false, 0
			for _, line := range strings.Split(pane, "\n") {
				if seen, _ := scanBackground(line); !seen {
					unpainted++
				}
				if strings.Contains(line, sel) {
					selRows++
					if strings.Contains(line, "› ") {
						chevronRow = true
					}
				}
			}
			if unpainted != 0 {
				t.Errorf("theme=%c group=%v: %d list lines carry no background at all — the paint pass must fill every cell",
					themeKey, group, unpainted)
			}
			if selRows != 1 {
				t.Errorf("theme=%c group=%v: %d list lines carry the Sel highlight, want exactly 1 (the selected row)",
					themeKey, group, selRows)
			}
			if !chevronRow {
				t.Errorf("theme=%c group=%v: the Sel row does not show the \"› \" chevron", themeKey, group)
			}
		}
	}
}
