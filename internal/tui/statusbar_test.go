package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/ertugrulhaskan/engram/internal/config"
	"github.com/ertugrulhaskan/engram/internal/memory"
	"github.com/ertugrulhaskan/engram/internal/team"
)

// actionModel builds a model whose first (selected) memory is team-shared with
// the given scope project and injected sync state, wide enough that no hint
// truncates.
func actionModel(state team.SyncState, project string) Model {
	mems := sampleMemories()
	mems[0].Shared = memory.EngramMeta{ID: "m-1", Scope: "team", Project: project}
	m := New(mems, samplePlans(), nil, config.Config{})
	if state != team.StateNone {
		m.syncStates = map[string]team.SyncState{mems[0].Path: state}
	}
	m.rebuildRows()
	var cur tea.Model = m
	cur, _ = cur.Update(tea.WindowSizeMsg{Width: 140, Height: 30})
	return cur.(Model)
}

// bottomLine returns the rendered status bar with styling stripped. The footer
// is a 3-row block (padding, bar, padding), so the bar is the second-to-last line.
func bottomLine(m Model) string {
	lines := strings.Split(m.View(), "\n")
	return ansi.Strip(lines[len(lines)-2])
}

// TestStatusBarOffersActionPerState pins offeredAction end to end: the status
// bar leads with the one key that works for the selected row's state — and
// never advertises a team key that would be dead.
func TestStatusBarOffersActionPerState(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cases := []struct {
		name    string
		state   team.SyncState
		project string
		want    string   // leading offered hint
		absent  []string // verbs that must not appear
	}{
		{"behind+project offers pull", team.StateIncoming, "acme/app", "p  pull", []string{"resolve", "promote"}},
		{"behind+global offers pull too, now that pull reconciles globals", team.StateIncoming, "global", "p  pull", []string{"resolve", "promote"}},
		{"ahead offers promote", team.StateLocalAhead, "acme/app", "P  promote", []string{"pull", "resolve"}},
		{"missing offers promote", team.StateMissing, "acme/app", "P  promote", []string{"pull", "resolve"}},
		{"conflict offers resolve", team.StateDiverged, "acme/app", "r  resolve", []string{"pull", "promote"}},
		{"unknown offers resolve", team.StateDiffers, "acme/app", "r  resolve", []string{"pull", "promote"}},
	}
	for _, c := range cases {
		bar := bottomLine(actionModel(c.state, c.project))
		if !strings.HasPrefix(bar, "  "+c.want) {
			t.Errorf("%s: bar %q does not lead with %q", c.name, bar, c.want)
		}
		for _, verb := range c.absent {
			if strings.Contains(bar, verb) {
				t.Errorf("%s: bar %q advertises dead verb %q", c.name, bar, verb)
			}
		}
	}
}

// TestStatusBarPersonalRowHasNoTeamKeys: a row with nothing to sync shows no
// team verbs at all.
func TestStatusBarPersonalRowHasNoTeamKeys(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	bar := bottomLine(actionModel(team.StateNone, ""))
	for _, verb := range []string{"pull", "promote", "resolve"} {
		if strings.Contains(bar, verb) {
			t.Errorf("personal row's bar %q advertises %q", bar, verb)
		}
	}
}

// TestGroupToggleHintNamesTarget: the g hint names the grouping you would get,
// not the one you have.
func TestGroupToggleHintNamesTarget(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := actionModel(team.StateNone, "")
	if bar := bottomLine(m); !strings.Contains(bar, "group by type") {
		t.Errorf("grouped by project, bar %q should offer %q", bar, "group by type")
	}
	var cur tea.Model = m
	cur, _ = cur.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")})
	if bar := bottomLine(cur.(Model)); !strings.Contains(bar, "group by project") {
		t.Errorf("grouped by type, bar %q should offer %q", bar, "group by project")
	}
}

// TestCtrlKPaletteAlias: ctrl+k opens the palette from normal mode, while
// inside the palette it keeps meaning "move up".
func TestCtrlKPaletteAlias(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var cur tea.Model = actionModel(team.StateNone, "")
	cur, _ = cur.Update(tea.KeyMsg{Type: tea.KeyCtrlK})
	if got := cur.(Model).mode; got != modePalette {
		t.Fatalf("ctrl+k mode = %v, want modePalette", got)
	}
	cur, _ = cur.Update(tea.KeyMsg{Type: tea.KeyDown})
	if got := cur.(Model).palCursor; got != 1 {
		t.Fatalf("palette down: cursor = %d, want 1", got)
	}
	cur, _ = cur.Update(tea.KeyMsg{Type: tea.KeyCtrlK})
	m := cur.(Model)
	if m.mode != modePalette || m.palCursor != 0 {
		t.Errorf("ctrl+k inside palette: mode=%v cursor=%d, want palette/0 (move up)", m.mode, m.palCursor)
	}
}

// TestAtKeyLaunchesAssistant: @ is bound in normal mode (it was advertised in
// the files hints but did nothing). With no claude binary the handler must
// answer with its not-found message — proof the key reaches the assistant path.
func TestAtKeyLaunchesAssistant(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	orig := lookClaude
	lookClaude = func() string { return "" }
	defer func() { lookClaude = orig }()

	var cur tea.Model = actionModel(team.StateNone, "")
	cur, _ = cur.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("@")})
	if got := cur.(Model).status; !strings.Contains(got, "claude CLI not found") {
		t.Errorf("@ did not reach the assistant handler; status = %q", got)
	}
}
