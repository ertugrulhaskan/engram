package tui

import (
	"os"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ertugrulhaskan/engram/internal/config"
)

func TestConfigAppliesTheme(t *testing.T) {
	// The stable config key and the display name both resolve.
	if got := New(sampleMemories(), nil, nil, config.Config{Theme: "paperback"}).theme().Name; got != "Paperback" {
		t.Errorf("theme = %q, want Paperback", got)
	}
	if got := New(sampleMemories(), nil, nil, config.Config{Theme: "CRT"}).theme().Name; got != "CRT" {
		t.Errorf("theme = %q, want CRT", got)
	}
	// Unknown theme name falls back to the default.
	if got := New(nil, nil, nil, config.Config{Theme: "Nope"}).theme().Name; got != themes[0].Name {
		t.Errorf("unknown theme = %q, want default %q", got, themes[0].Name)
	}
}

// Configs written by pre-redesign engram versions name one of the five retired
// themes; they all land on Midnight instead of erroring or resetting oddly.
func TestLegacyThemeNamesMapToMidnight(t *testing.T) {
	for _, legacy := range []string{"Dracula", "Tokyo Night", "Nord", "Gruvbox", "Classic Dark"} {
		if got := New(nil, nil, nil, config.Config{Theme: legacy}).theme().Name; got != "Midnight" {
			t.Errorf("legacy theme %q = %q, want Midnight", legacy, got)
		}
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
// returning to normal mode. The seed persists the stable theme key, not the
// display name.
func TestPaletteSettingsOpensConfigFile(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m, cmd := openSettings(t, New(sampleMemories(), nil, nil, config.Config{Theme: "paperback"}))
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
	if got := config.Load(); got.Theme != "paperback" {
		t.Errorf("seeded config theme = %q, want paperback", got.Theme)
	}
}

// After editing the config file, closing the editor re-reads it and applies the
// new theme + editor (rather than treating it as a memory).
func TestSettingsFileReloadApplies(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var m tea.Model = New(sampleMemories(), nil, nil, config.Config{})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	p, _ := config.Path()
	if err := config.Save(config.Config{Theme: "crt", Editor: "code --wait"}); err != nil {
		t.Fatal(err)
	}
	m, _ = m.Update(editorFinishedMsg{path: p})
	got := m.(Model)
	if got.theme().Name != "CRT" {
		t.Errorf("theme after config edit = %q, want CRT", got.theme().Name)
	}
	if got.editorOverride != "code --wait" {
		t.Errorf("editor after config edit = %q, want 'code --wait'", got.editorOverride)
	}
}

func TestThemeSwitchPersists(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var m tea.Model = New(sampleMemories(), nil, nil, config.Config{})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("3")}) // CRT
	if got := m.(Model).theme().Name; got != "CRT" {
		t.Fatalf("after key 3, theme=%q want CRT", got)
	}
	if got := config.Load(); got.Theme != "crt" {
		t.Errorf("persisted theme = %q, want crt", got.Theme)
	}
}

// A hand-edited config with an unknown theme keeps the current theme on reload
// instead of resetting to the default (and re-persisting over the user's file).
func TestSettingsReloadUnknownThemeKeepsCurrent(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var m tea.Model = New(sampleMemories(), nil, nil, config.Config{Theme: "crt"})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	p, _ := config.Path()
	if err := config.Save(config.Config{Theme: "Nope"}); err != nil {
		t.Fatal(err)
	}
	m, _ = m.Update(editorFinishedMsg{path: p})
	if got := m.(Model).theme().Name; got != "CRT" {
		t.Errorf("theme after bad config edit = %q, want CRT (unchanged)", got)
	}
	if got := config.Load().Theme; got != "Nope" {
		t.Errorf("config rewritten to %q — an unknown value must be left alone", got)
	}
}

// Switching themes must not wipe unrelated settings from config.json (the save
// round-trips the file instead of writing a fresh Theme+Editor-only Config).
func TestThemeSwitchKeepsScanSettings(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := config.Save(config.Config{Theme: "midnight", SecretScanAction: "warn", SecretScanScope: "secrets+pii"}); err != nil {
		t.Fatal(err)
	}
	var m tea.Model = New(sampleMemories(), nil, nil, config.Load())
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("2")})
	got := config.Load()
	if got.Theme != "paperback" {
		t.Fatalf("persisted theme = %q, want paperback", got.Theme)
	}
	if got.SecretScanAction != "warn" || got.SecretScanScope != "secrets+pii" {
		t.Errorf("scan settings lost on theme switch: %+v", got)
	}
}
