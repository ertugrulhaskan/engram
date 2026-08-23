package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/ertugrulhaskan/engram/internal/config"
)

func TestHelpOverlay(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	const w, h = 100, 30
	var m tea.Model = New(sampleMemories(), samplePlans(), nil, config.Config{}).WithVersion("v9.9.9")
	m, _ = m.Update(tea.WindowSizeMsg{Width: w, Height: h})

	// `?` opens the help overlay.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	if got := m.(Model).mode; got != modeHelp {
		t.Fatalf("after `?`, mode = %v, want modeHelp", got)
	}
	out := m.(Model).View()
	for _, want := range []string{"Keybindings", "palette", "team:", "engram v9.9.9", "MIT"} {
		if !strings.Contains(out, want) {
			t.Errorf("help overlay missing %q", want)
		}
	}
	for _, line := range strings.Split(out, "\n") {
		if lw := lipgloss.Width(line); lw > w {
			t.Errorf("help line wider than %d (=%d): %q", w, lw, line)
		}
	}

	if testing.Verbose() {
		t.Logf("\n%s\n", out)
	}

	// Any key dismisses it.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if got := m.(Model).mode; got != modeNormal {
		t.Fatalf("after dismiss, mode = %v, want modeNormal", got)
	}
}

// The `?` overlay has to survive clampFrameHeight at 29 rows — the height it fit
// exactly before `a` was documented. Adding a row to helpGroups is a one-line
// change that silently costs the bottom border here, and TestHelpOverlay only
// covers H=30, so it would not notice. This pins the tighter height.
func TestHelpOverlayFitsAt29Rows(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	const h = 29
	var m tea.Model = New(sampleMemories(), samplePlans(), nil, config.Config{}).WithVersion("v9.9.9")
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: h})

	got := ansi.Strip(clampFrameHeight(m.(Model).helpModal(), h))
	if !strings.Contains(got, "╰") {
		t.Errorf("help overlay lost its bottom border at H=%d:\n%s", h, got)
	}
	if !strings.Contains(got, "MIT") {
		t.Errorf("help overlay lost its about footer at H=%d:\n%s", h, got)
	}
}
