package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"github.com/ertugrulhaskan/engram/internal/config"
)

// stripLine extracts the source-strip row (second chrome line) from a rendered
// frame, with all styling removed.
func stripLine(frame string) string {
	lines := strings.Split(frame, "\n")
	if len(lines) < 2 {
		return ""
	}
	return ansi.Strip(lines[1])
}

func key(r string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(r)} }

// TestSourceStripContents pins the strip's information: every source with its
// live count on the left, and the list-shaping echo (group/type, search) or the
// palette hint on the right.
func TestSourceStripContents(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var cur tea.Model = New(sampleMemories(), samplePlans(), sampleDocs(), config.Config{})
	cur, _ = cur.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	got := stripLine(cur.(Model).View())
	for _, want := range []string{"memories 5", "plans 2", "files 2", "group project · type all"} {
		if !strings.Contains(got, want) {
			t.Errorf("strip %q missing %q", got, want)
		}
	}

	// Shaping the memories list updates the echo.
	cur, _ = cur.Update(key("t")) // type: all → user
	cur, _ = cur.Update(key("g")) // group: project → type
	got = stripLine(cur.(Model).View())
	if !strings.Contains(got, "group type · type user") {
		t.Errorf("strip %q missing shaped echo %q", got, "group type · type user")
	}

	// A committed search is echoed so the narrowed list has a visible reason.
	cur, _ = cur.Update(key("/"))
	cur, _ = cur.Update(key("p"))
	cur, _ = cur.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got = stripLine(cur.(Model).View())
	if !strings.Contains(got, "“p”") {
		t.Errorf("strip %q missing search echo", got)
	}

	// Sources without shaping state point at the palette instead (^K since the
	// ctrl+k alias landed with the contextual status bar).
	cur, _ = cur.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	got = stripLine(cur.(Model).View())
	if !strings.Contains(got, "^K jump or run") {
		t.Errorf("plans strip %q missing palette hint", got)
	}
}

// TestSourceStripActiveTab pins which tab carries the active treatment (Fg bold
// underline label + Accent count) and that the rest stay Faint — under
// TrueColor, since the assertion is about SGR styling.
func TestSourceStripActiveTab(t *testing.T) {
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(old)

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var cur tea.Model = New(sampleMemories(), samplePlans(), sampleDocs(), config.Config{})
	cur, _ = cur.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	th := cur.(Model).theme()

	active := func(label string) string {
		return onbg(th.Fg, th.Bg).Bold(true).Underline(true).Render(label)
	}
	inactive := func(s string) string { return onbg(th.Faint, th.Bg).Render(s) }

	frame := cur.(Model).View()
	if !strings.Contains(frame, active("memories")) {
		t.Error("active memories tab not rendered Fg+bold+underline")
	}
	if !strings.Contains(frame, onbg(th.Accent, th.Bg).Underline(true).Render("5")) {
		t.Error("active tab count not rendered in Accent+underline")
	}
	if !strings.Contains(frame, inactive("plans 2")) || !strings.Contains(frame, inactive("files 2")) {
		t.Error("inactive tabs not rendered Faint")
	}

	cur, _ = cur.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	frame = cur.(Model).View()
	if !strings.Contains(frame, active("plans")) {
		t.Error("after shift+tab the plans tab is not the active one")
	}
	if !strings.Contains(frame, inactive("memories 5")) {
		t.Error("after shift+tab the memories tab did not go Faint")
	}
}

// TestShiftTabCyclesAndPersistsState guards the switch semantics: shift+tab
// cycles memories → plans → files → memories, and the per-source cursor, type
// filter, and group mode all survive the round trip (deviation 3: a superset of
// the spec's reset-to-0).
func TestShiftTabCyclesAndPersistsState(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var cur tea.Model = New(sampleMemories(), samplePlans(), sampleDocs(), config.Config{})
	cur, _ = cur.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	cur, _ = cur.Update(key("t")) // type filter: user
	cur, _ = cur.Update(key("g")) // group by type
	cur, _ = cur.Update(key("j")) // move off the first row
	before := cur.(Model)

	wantOrder := []srcKind{srcPlans, srcFiles, srcMemories}
	for _, want := range wantOrder {
		cur, _ = cur.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
		if got := cur.(Model).srcKind; got != want {
			t.Fatalf("shift+tab landed on source %v, want %v", got, want)
		}
	}
	after := cur.(Model)
	if after.cursor != before.cursor {
		t.Errorf("cursor not remembered across the cycle: %d, want %d", after.cursor, before.cursor)
	}
	if after.typeIdx != before.typeIdx {
		t.Errorf("type filter reset across the cycle: %d, want %d", after.typeIdx, before.typeIdx)
	}
	if after.groupBy != before.groupBy {
		t.Errorf("group mode reset across the cycle: %v, want %v", after.groupBy, before.groupBy)
	}
}

// TestShiftTabClearsSearchAndWorksFromPreview: the search is per-source and
// clears on switch (today's behavior, kept), and the key works with the
// preview pane focused, not just the list.
func TestShiftTabClearsSearchAndWorksFromPreview(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var cur tea.Model = New(sampleMemories(), samplePlans(), sampleDocs(), config.Config{})
	cur, _ = cur.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	cur, _ = cur.Update(key("/"))
	cur, _ = cur.Update(key("dev"))
	cur, _ = cur.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if got := cur.(Model).search.Value(); got != "dev" {
		t.Fatalf("committed search = %q, want %q", got, "dev")
	}
	cur, _ = cur.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	if got := cur.(Model).search.Value(); got != "" {
		t.Errorf("search survived a source switch: %q", got)
	}

	cur, _ = cur.Update(tea.KeyMsg{Type: tea.KeyTab}) // focus the preview
	if cur.(Model).focus != focusPreview {
		t.Fatal("tab did not focus the preview")
	}
	from := cur.(Model).srcKind
	cur, _ = cur.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	if cur.(Model).srcKind == from {
		t.Error("shift+tab did not switch source while the preview was focused")
	}
	if cur.(Model).focus != focusPreview {
		t.Error("source switch stole the preview focus")
	}
}
