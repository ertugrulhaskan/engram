package tui

import (
	"os"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ertugrulhaskan/engram/internal/config"
)

func TestConfigAppliesTheme(t *testing.T) {
	if got := New(sampleMemories(), nil, nil, config.Config{Theme: "Nord"}).theme().Name; got != "Nord" {
		t.Errorf("theme = %q, want Nord", got)
	}
	// Unknown theme name falls back to the default.
	if got := New(nil, nil, nil, config.Config{Theme: "Nope"}).theme().Name; got != themes[0].Name {
		t.Errorf("unknown theme = %q, want default %q", got, themes[0].Name)
	}
}

func TestConfigEditorOverride(t *testing.T) {
	m := New(nil, nil, nil, config.Config{Editor: "code --wait"})
	if got := m.resolveEditor(); len(got) != 2 || got[0] != "code" || got[1] != "--wait" {
		t.Errorf("resolveEditor = %v, want [code --wait]", got)
	}
}

// openSettings drives Ctrl+P → /settings → Enter and returns the model + the
// command it produced (the editor launch).
func openSettings(t *testing.T, m tea.Model) (tea.Model, tea.Cmd) {
	t.Helper()
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	m = typeRunes(m, "/settings")
	return m.Update(tea.KeyMsg{Type: tea.KeyEnter})
}

// /settings seeds the config file (when missing) and opens it in the editor,
// returning to normal mode.
func TestPaletteSettingsOpensConfigFile(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m, cmd := openSettings(t, New(sampleMemories(), nil, nil, config.Config{Theme: "Nord"}))
	if got := m.(Model).mode; got != modeNormal {
		t.Fatalf("/settings should return to normal mode, got %v", got)
	}
	if cmd == nil {
		t.Error("/settings did not return an editor command")
	}
	p, _ := config.Path()
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("config file not created: %v", err)
	}
	if got := config.Load(); got.Theme != "Nord" {
		t.Errorf("seeded config theme = %q, want Nord", got.Theme)
	}
}

// After editing the config file, closing the editor re-reads it and applies the
// new theme + editor (rather than treating it as a memory).
func TestSettingsFileReloadApplies(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var m tea.Model = New(sampleMemories(), nil, nil, config.Config{})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	p, _ := config.Path()
	if err := config.Save(config.Config{Theme: "Nord", Editor: "code --wait"}); err != nil {
		t.Fatal(err)
	}
	m, _ = m.Update(editorFinishedMsg{path: p})
	got := m.(Model)
	if got.theme().Name != "Nord" {
		t.Errorf("theme after config edit = %q, want Nord", got.theme().Name)
	}
	if got.editorOverride != "code --wait" {
		t.Errorf("editor after config edit = %q, want 'code --wait'", got.editorOverride)
	}
}

func TestThemeSwitchPersists(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var m tea.Model = New(sampleMemories(), nil, nil, config.Config{})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("3")}) // Nord
	if got := m.(Model).theme().Name; got != "Nord" {
		t.Fatalf("after key 3, theme=%q want Nord", got)
	}
	if got := config.Load(); got.Theme != "Nord" {
		t.Errorf("persisted theme = %q, want Nord", got.Theme)
	}
}
