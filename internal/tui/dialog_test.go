package tui

import (
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ertugrulhaskan/engram/internal/config"
)

var sgr = regexp.MustCompile("\x1b\\[[0-9;]*m")

// TestDialogBoxesUniformWidth guards against a row rendering wider than the box
// frame — e.g. the palette input overflowing by the cursor cell, which clampFrame
// would otherwise hide by truncating the border. Every line of a framed dialog
// must be exactly boxWidth+2 (content + the two border columns). Dialogs are
// opened through their real key flows so width-setup (e.g. the new-memory input)
// matches production.
func TestDialogBoxesUniformWidth(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	runes := func(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }
	for _, w := range []int{64, 80, 100, 140} {
		var tm tea.Model = New(sampleMemories(), samplePlans(), nil, config.Config{})
		tm, _ = tm.Update(tea.WindowSizeMsg{Width: w, Height: 30})
		base := tm.(Model)
		cw := base.boxWidth()
		open := func(k tea.KeyMsg) Model { m, _ := base.Update(k); return m.(Model) }

		boxes := map[string]string{
			"palette": open(tea.KeyMsg{Type: tea.KeyCtrlP}).paletteBox(),
			"new":     open(runes("n")).newModal(),
			"confirm": open(runes("d")).confirmModal(),
			"help":    open(runes("?")).helpModal(),
		}
		for name, box := range boxes {
			for i, ln := range strings.Split(box, "\n") {
				if lw := lipgloss.Width(ln); lw != cw+2 {
					t.Errorf("w=%d %s line %d width=%d, want %d: %q",
						w, name, i, lw, cw+2, sgr.ReplaceAllString(ln, ""))
				}
			}
		}
	}
}
