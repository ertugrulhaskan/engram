package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// styled returns a string with SGR escapes around `text`, padded with plain
// spaces to a known visible width — exercises cutLeft's CSI handling.
func styled(text string, width int) string {
	s := "\x1b[1;38;2;10;20;30;48;2;40;50;60m" + text + "\x1b[0m"
	if pad := width - len(text); pad > 0 {
		s += strings.Repeat(" ", pad)
	}
	return s
}

// cutLeft must drop exactly cutWidth visible cells regardless of embedded SGR
// sequences. Regression: the CSI introducer '[' was once treated as a terminator,
// so the numeric SGR parameters were counted as visible width.
func TestCutLeftWidth(t *testing.T) {
	line := styled("HELLO", 20) // 5 visible + 15 spaces = width 20
	for _, cut := range []int{0, 3, 5, 8, 20, 25} {
		want := 20 - cut
		if want < 0 {
			want = 0
		}
		if got := lipgloss.Width(cutLeft(line, cut)); got != want {
			t.Errorf("cutLeft(_, %d) width = %d, want %d", cut, got, want)
		}
	}
}

// placeOverlay must keep every composited row exactly the background width, even
// when the background lines are heavily styled.
func TestPlaceOverlayUniformWidth(t *testing.T) {
	bg := strings.Join([]string{styled("alpha", 30), styled("beta", 30), styled("gamma", 30)}, "\n")
	fg := "BOXXX\nBOXXX"
	out := placeOverlay(10, 0, fg, bg)
	for i, ln := range strings.Split(out, "\n") {
		if w := lipgloss.Width(ln); w != 30 {
			t.Errorf("overlaid line %d width = %d, want 30", i, w)
		}
	}
}

// widthInvariant fails if any rendered line is wider than w.
func widthInvariant(t *testing.T, out string, w int) {
	t.Helper()
	for _, line := range strings.Split(out, "\n") {
		if got := lipgloss.Width(line); got > w {
			t.Errorf("line wider than %d (=%d): %q", w, got, line)
		}
	}
}

// The palette floats over the panes: the box is drawn while a known list item is
// still visible behind it, and nothing overflows the terminal width.
func TestPaletteFloatsOverPanes(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var m tea.Model = ready(t)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	out := m.(Model).View()

	if !strings.Contains(out, "Type / for commands") {
		t.Error("palette search header not rendered")
	}
	if !strings.Contains(out, "Browse memories, plans, files") {
		t.Error("palette guide rows not rendered")
	}
	// The box is top-anchored and centered, so it covers the right of the early
	// list rows; their left edge must still show through (proving a float, not a
	// pane replacement, which would drop the list entirely). The "webapp" group
	// header sits left of the box and survives.
	if !strings.Contains(out, "webapp") {
		t.Error("list content not visible behind the floating palette")
	}
	widthInvariant(t, out, 100)
}

// The delete confirm floats the same way: the highlighted target row shows over
// the still-visible list/preview, and nothing overflows the width.
func TestConfirmFloatsOverPanes(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var m tea.Model = ready(t)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	if m.(Model).mode != modeConfirm {
		t.Fatalf("d did not open the confirm dialog (mode=%v)", m.(Model).mode)
	}
	out := m.(Model).View()
	if !strings.Contains(out, "Delete memory?") {
		t.Error("delete dialog header not rendered")
	}
	if !strings.Contains(out, "webapp") {
		t.Error("list content not visible behind the floating confirm dialog")
	}
	widthInvariant(t, out, 100)
}
