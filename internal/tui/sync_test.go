package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"github.com/ertugrulhaskan/engram/internal/config"
	"github.com/ertugrulhaskan/engram/internal/memory"
	"github.com/ertugrulhaskan/engram/internal/team"
)

// TestSyncVocabulary pins the spec's display words — StateIncoming reads
// "behind" and StateDiffers "unknown" (the Go enum names stay) — and the
// semantic color each state maps to.
func TestSyncVocabulary(t *testing.T) {
	th := themes[0]
	cases := []struct {
		state team.SyncState
		word  string
		color string
	}{
		{team.StateSynced, "synced", th.OK},
		{team.StateIncoming, "behind", th.Info},
		{team.StateLocalAhead, "ahead", th.Warn},
		{team.StateDiverged, "conflict", th.Danger},
		{team.StateDiffers, "unknown", th.Faint},
		{team.StateMissing, "missing", th.Danger},
		{team.StateNone, "", ""},
	}
	for _, c := range cases {
		word, color := th.syncBadge(c.state)
		if word != c.word || color != c.color {
			t.Errorf("syncBadge(%v) = (%q, %q), want (%q, %q)", c.state, word, color, c.word, c.color)
		}
	}
}

// syncedModel builds a model whose first memory is team-shared (global scope)
// with the given sync state injected, sized and ready to render.
func syncedModel(state team.SyncState, extra map[int]team.SyncState) Model {
	mems := sampleMemories()
	mems[0].Shared = memory.EngramMeta{ID: "m-1", Scope: "team", Project: "global"}
	m := New(mems, samplePlans(), nil, config.Config{})
	states := map[string]team.SyncState{mems[0].Path: state}
	for i, s := range extra {
		states[mems[i].Path] = s
	}
	m.syncStates = states
	m.rebuildRows()
	var cur tea.Model = m
	cur, _ = cur.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	return cur.(Model)
}

// TestSyncPillBracketed: the list renders sync state as an outlined bracketed
// word (no filled background), and the preview meta shows the bracketed scope
// pill plus the state word.
func TestSyncPillBracketed(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := syncedModel(team.StateIncoming, map[int]team.SyncState{1: team.StateDiffers})

	plain := ansi.Strip(m.View())
	for _, want := range []string{"[behind]", "[unknown]", "team [global] · behind"} {
		if !strings.Contains(plain, want) {
			t.Errorf("frame missing %q", want)
		}
	}
	// Group headers keep their own cycled color — the sync color must not leak
	// into (or blank out) GroupColor via variable reuse in memoryItems.
	for _, r := range m.rows {
		if r.kind == rowHeader && r.color == "" {
			t.Errorf("group header %q lost its color", r.label)
		}
	}
	// The retired words must be gone ("↑↓/jk" in the hints keeps arrow glyphs
	// legitimate, so only the words are checked).
	for _, gone := range []string{"incoming", "differs"} {
		if strings.Contains(plain, gone) {
			t.Errorf("frame still contains retired %q", gone)
		}
	}
	if testing.Verbose() {
		fmt.Printf("\n========== sync pills (behind + unknown) ==========\n%s\n", m.View())
	}
}

// TestSyncPillStyling: under TrueColor the selected row's pill is bold in the
// state color over the Sel highlight; an unselected row's pill is the plain
// state color with no filled pill background.
func TestSyncPillStyling(t *testing.T) {
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(old)

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := syncedModel(team.StateIncoming, nil) // single shared row → syncW == len("[behind]")
	th := m.theme()

	frame := m.View()
	selPill := fg(th.Info).Background(lipgloss.Color(th.Sel)).Bold(true).Render("[behind]")
	if !strings.Contains(frame, selPill) {
		t.Error("selected row's pill is not bold state-color over the Sel highlight")
	}

	// Move the cursor off the shared row: the pill loses bold and the highlight.
	var cur tea.Model = m
	cur, _ = cur.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	frame = cur.(Model).View()
	plainPill := fg(th.Info).Render("[behind]")
	if !strings.Contains(frame, plainPill) {
		t.Error("unselected row's pill is not plain state-colored text")
	}
	if strings.Contains(frame, selPill) {
		t.Error("unselected row's pill kept the selected treatment")
	}
}
