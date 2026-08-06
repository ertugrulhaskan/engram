package tui

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"github.com/ertugrulhaskan/engram/internal/config"
	"github.com/ertugrulhaskan/engram/internal/memory"
	"github.com/ertugrulhaskan/engram/internal/team"
)

var errStoreTimeTest = errors.New("store time lookup failed")

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

// TestSyncStripPerState pins the preview band: the spec sentence, the offered
// action chip, the gauge, and the honest per-state stamp.
func TestSyncStripPerState(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cases := []struct {
		name    string
		state   team.SyncState
		project string
		want    []string
		absent  []string
	}{
		{"behind", team.StateIncoming, "acme/app",
			[]string{"The team copy moved ahead. Yours is untouched.", "[p pull]", "you ▬▬▬ ← ▬▬▬ team"},
			[]string{"store advanced"}}, // no stamp until git has answered
		{"synced", team.StateSynced, "acme/app",
			[]string{"In sync with the team store.", "you ▬▬▬ = ▬▬▬ team", "edited "},
			[]string{"[p pull]", "[P promote]", "[r resolve]"}},
		{"conflict", team.StateDiverged, "acme/app",
			[]string{"Both sides changed since you last synced.", "[r resolve]", "↔", "diverged "},
			nil},
		{"missing", team.StateMissing, "acme/app",
			[]string{"Promoted once, but it is not in the store anymore.", "[P promote]", "✕", "not in store"},
			nil},
		{"unknown", team.StateDiffers, "acme/app",
			[]string{"Shared before sync tracking existed, so there is no direction.", "[r resolve]", "no anchor"},
			nil},
	}
	for _, c := range cases {
		plain := ansi.Strip(actionModel(c.state, c.project).View())
		for _, w := range c.want {
			if !strings.Contains(plain, w) {
				t.Errorf("%s: frame missing %q", c.name, w)
			}
		}
		for _, a := range c.absent {
			if strings.Contains(plain, a) {
				t.Errorf("%s: frame unexpectedly contains %q", c.name, a)
			}
		}
	}
	// Personal rows get no band at all.
	plain := ansi.Strip(actionModel(team.StateNone, "").View())
	if strings.Contains(plain, "In sync") || strings.Contains(plain, "you ▬▬▬") {
		t.Error("personal row rendered a sync strip")
	}
}

// TestSyncStripStoreStamp covers the lazy "store advanced" stamp: the fetch is
// marked as asked on selection, the stamp appears only after git answers, and a
// failed lookup stays omitted.
func TestSyncStripStoreStamp(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := actionModel(team.StateIncoming, "acme/app")
	if !m.storeTimeAsked["m-1"] {
		t.Fatal("selecting a behind row did not mark its store time as asked")
	}

	var cur tea.Model = m
	cur, _ = cur.Update(storeTimeMsg{id: "m-1", t: time.Now().Add(-2 * time.Hour)})
	if plain := ansi.Strip(cur.(Model).View()); !strings.Contains(plain, "store advanced 2h ago") {
		t.Error("fetched store time did not render as the stamp")
	}

	m2 := actionModel(team.StateIncoming, "acme/app")
	cur = m2
	cur, _ = cur.Update(storeTimeMsg{id: "m-1", err: errStoreTimeTest})
	if plain := ansi.Strip(cur.(Model).View()); strings.Contains(plain, "store advanced") {
		t.Error("failed lookup still rendered a stamp")
	}
}

// TestSyncStripViewportShrink: the viewport gives up exactly the band's rows,
// and gets them back on a personal row.
func TestSyncStripViewportShrink(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := actionModel(team.StateIncoming, "acme/app")
	if got, want := m.viewport.Height, m.panesH-7; got != want {
		t.Errorf("behind row: viewport height %d, want %d (band shown)", got, want)
	}
	var cur tea.Model = m
	cur, _ = cur.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if got, want := cur.(Model).viewport.Height, m.panesH-4; got != want {
		t.Errorf("personal row: viewport height %d, want %d (band hidden)", got, want)
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
