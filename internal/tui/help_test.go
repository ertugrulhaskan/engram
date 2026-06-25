package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

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
	for _, want := range []string{"Keybindings", "command palette", "engram v9.9.9", "MIT"} {
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
