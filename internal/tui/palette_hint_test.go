package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/ertugrulhaskan/engram/internal/config"
)

// The header hint must never squeeze the input below its placeholder. Under a
// fixed 64-cell cutoff, 76- and 77-column terminals showed the hint beside an
// 18-19 cell input, and bubbles 1.0's textinput cut the placeholder to
// "type to jump or r…". The sweep checks the rendered header at every width:
//   - the frame never overflows;
//   - wherever the box can hold the placeholder at all, it is shown whole —
//     the cutoff is read off each box, not hardcoded, so it follows boxWidth's
//     floor, the header chrome and the placeholder text if any of them move;
//   - a shown hint never costs the placeholder its tail;
//   - once the hint appears it stays for every wider terminal, and it is shown
//     on a standard 80-column terminal, so it cannot quietly switch off.
func TestPaletteHintNeverTruncatesPlaceholder(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	pw := lipgloss.Width(palPlaceholder)
	sawHint := false
	for w := 20; w <= 140; w++ {
		var tm tea.Model = New(sampleMemories(), samplePlans(), sampleDocs(), config.Config{})
		tm, _ = tm.Update(tea.WindowSizeMsg{Width: w, Height: 30})
		tm, _ = tm.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
		m := tm.(Model)
		cw := m.boxWidth()
		lines := strings.Split(m.paletteBox(), "\n")
		for i, ln := range lines {
			if lw := lipgloss.Width(ln); lw != cw+2 {
				t.Errorf("width %d: palette line %d is %d cells, want %d", w, i, lw, cw+2)
			}
		}
		header := ansi.Strip(lines[1]) // line 0 is the top border
		whole := strings.Contains(header, palPlaceholder)
		shown := strings.Contains(header, palHint)
		room := cw-palInputChrome >= pw // the box fits the placeholder without the hint
		switch {
		case room && !whole:
			t.Errorf("width %d (box %d): room for the placeholder but header reads %q", w, cw, header)
		case shown && !whole:
			t.Errorf("width %d (box %d): hint squeezed the placeholder: %q", w, cw, header)
		case sawHint && !shown:
			t.Errorf("width %d (box %d): hint disappeared after showing at a narrower width", w, cw)
		}
		sawHint = sawHint || shown
		if w == 80 && !shown {
			t.Errorf("80 columns (box %d): hint not shown: %q", cw, header)
		}
	}
}
