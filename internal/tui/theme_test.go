package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/ertugrulhaskan/engram/internal/config"
)

// TestThemeSwitchRerendersPreviewOnce guards the press-twice bug: a single
// theme keypress must re-render the preview body with the new theme's glamour
// style. setTheme used to clear the preview cache and rebuild rows before
// rebuilding the renderer — rebuildRows ends in syncPreview, so the cache was
// refilled with a stale-theme render and the preview only caught up on the
// next press.
func TestThemeSwitchRerendersPreviewOnce(t *testing.T) {
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(old)

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var cur tea.Model = New(sampleMemories(), samplePlans(), nil, config.Config{})
	cur, _ = cur.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	// Positive control: Midnight's frame carries the dracula glamour body color
	// (also Midnight's Fg), so its absence later is meaningful.
	const dracBody = "38;2;248;248;242"
	if !strings.Contains(cur.(Model).View(), dracBody) {
		t.Fatal("control failed: Midnight frame does not carry the dracula body color")
	}

	// One press of '2' → Paperback. No Paperback token is #f8f8f2, so any
	// remaining occurrence is a stale dracula-rendered preview body.
	cur, _ = cur.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("2")})
	if strings.Contains(cur.(Model).View(), dracBody) {
		t.Error("preview still rendered with the previous theme after one keypress")
	}
}
