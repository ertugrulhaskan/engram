package tui

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ertugrulhaskan/engram/internal/config"
	"github.com/ertugrulhaskan/engram/internal/secrets"
	"github.com/ertugrulhaskan/engram/internal/team"
)

var sgr = regexp.MustCompile("\x1b\\[[0-9;]*m")

// TestDialogBoxesUniformWidth guards against a row rendering wider than the box
// frame — e.g. the palette input overflowing by the cursor cell, which clampFrame
// would otherwise hide by truncating the border. Every line of a framed dialog
// must be exactly boxWidth+2 (content + the two border columns). Dialogs are
// opened through their real key flows so width-setup (e.g. the new-memory input)
// matches production.
func TestDialogBoxesUniformWidth(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	runes := func(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }
	for _, w := range []int{64, 80, 100, 140} {
		var tm tea.Model = New(sampleMemories(), samplePlans(), nil, config.Config{})
		tm, _ = tm.Update(tea.WindowSizeMsg{Width: w, Height: 30})
		base := tm.(Model)
		cw := base.boxWidth()
		open := func(k tea.KeyMsg) Model { m, _ := base.Update(k); return m.(Model) }

		// Team/drift dialogs can't be opened through their key flows without a
		// live store, so their state is injected; they carry no inputs, so the
		// key-flow width-setup concern doesn't apply to them.
		team1 := base
		team1.promoteTitle = "A memory title long enough to clip"
		team1.promoteKey = "github.com/acme/app"
		team1.secretFindings = []secrets.Finding{{Rule: "aws access key id", Line: 14, Match: "AKIA••••••••••••7Q2M"}}
		team1.withdrawTitle = "A shared memory"
		team1.pullPlan = team.PullResult{Placed: 1, Updated: 3, Ahead: 1, Conflicts: 1, UpToDate: 2, Skipped: 1}
		team1.setResolveSides(
			[]string{"Keep migrations reversible.", "Wrap every migration in a transaction."},
			[]string{"Keep migrations reversible.", "Wrap it, and keep a down step."},
			false)
		team1.driftUnindexed = []string{"migrations.md", "staging.md"}
		team1.driftDangling = []string{"gone.md"}

		boxes := map[string]string{
			"palette":  open(tea.KeyMsg{Type: tea.KeyCtrlP}).paletteBox(),
			"new":      open(runes("n")).newModal(),
			"confirm":  open(runes("d")).confirmModal(),
			"help":     open(runes("?")).helpModal(),
			"promote":  team1.scopeModal(),
			"secret":   team1.secretModal(),
			"withdraw": team1.withdrawModal(), // zero OwnerStatus — the taller caution variant
			"withdraw-verified": func() string {
				v := team1
				v.withdrawOwner = team.OwnerStatus{Owner: "a@example.com", Me: "a@example.com"}
				return v.withdrawModal()
			}(),
			"pull":      team1.pullModal(),
			"resolve":   team1.resolveModal(),
			"reconcile": team1.reconcileModal(),
		}
		for name, box := range boxes {
			for i, ln := range strings.Split(box, "\n") {
				if lw := lipgloss.Width(ln); lw != cw+2 {
					t.Errorf("w=%d %s line %d width=%d, want %d: %q",
						w, name, i, lw, cw+2, sgr.ReplaceAllString(ln, ""))
				}
			}
		}
	}
}

// TestResolveModalFitsFrame is the height counterpart to the width test above.
// A floating dialog has to fit the rows View actually paints — headerRows +
// panesH + footerRows, one short of the terminal, since reservedRows leaves the
// last row unwritten — and the resolve confirm is the one dialog that sizes
// itself, so it is the one that can get it wrong. It did, twice: budgeting
// against m.height made it a row too tall, and a floor of three diff rows kept
// it too tall on a short frame. Both cost the same row off the bottom — the
// footer, the only place the confirm says what enter does.
//
// The widths straddle 80 deliberately. The mechanics line is 61 cells against a
// text column of boxWidth()-4, so it wraps to a second row below 80 columns —
// the case a hand-counted chrome constant missed — and to a third at 42, which
// is what puts the floor at h=14: at 30-42 columns the modal cannot be smaller
// than 12 rows, and the alternative to that floor is dropping a row out of the
// shared dialog anatomy on narrow terminals only. Below it the box loses its
// bottom border, as every other dialog does far sooner at that size (help is 28
// rows against a 12-row frame there, pull 21).
func TestResolveModalFitsFrame(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	// A diff far longer than any budget, so the modal always wants its maximum.
	var yours, theirs []string
	for i := 0; i < 60; i++ {
		yours = append(yours, "yours line "+string(rune('a'+i%26)))
		theirs = append(theirs, "theirs line "+string(rune('a'+i%26)))
	}
	open := func(t *testing.T, w, h int) Model {
		t.Helper()
		var tm tea.Model = New(sampleMemories(), nil, nil, config.Config{})
		tm, _ = tm.Update(tea.WindowSizeMsg{Width: w, Height: h})
		m := tm.(Model)
		m.mode = modeResolveConfirm
		m.resolveTmp = "/tmp/merge.md"
		m.setResolveSides(yours, theirs, false)
		return m
	}
	fits := func(t *testing.T, m Model, what string) {
		t.Helper()
		if got := len(strings.Split(m.resolveModal(), "\n")); got > m.dialogRows() {
			t.Errorf("%s: modal is %d rows, a dialog can occupy %d (frame %d) — the bottom is clipped",
				what, got, m.dialogRows(), m.frameRows())
		}
		// The footer is the row a too-tall modal loses first, so assert the
		// rendered view still carries it rather than trusting the arithmetic.
		if !strings.Contains(sgr.ReplaceAllString(m.View(), ""), "open $EDITOR") {
			t.Errorf("%s: the resolve confirm's footer is missing from the view", what)
		}
	}

	// Every branch of the modal, not just the diff: the "nothing to show" ones
	// replace the legend and rows with a message, so they have their own height,
	// and resolveInvisible is reachable in ordinary use — a teammate on Windows
	// whose copy differs only in line endings is the case it was written for.
	for _, br := range []struct {
		name  string
		yours []string
		them  []string
		ident bool
	}{
		{"differs", yours, theirs, false},
		{"identical", yours, yours, true},
		{"invisible", yours, yours, false},
	} {
		for _, w := range []int{30, 42, 60, 76, 80, 100, 140} {
			for _, h := range []int{14, 16, 18, 20, 22, 24, 30, 50} {
				m := open(t, w, h)
				m.setResolveSides(br.yours, br.them, br.ident)
				fits(t, m, fmt.Sprintf("%s w=%d h=%d", br.name, w, h))
			}
		}
	}

	// Resizing with the confirm already open must re-budget: rows sized for the
	// old frame are exactly the too-tall dialog this budget exists to prevent.
	t.Run("resize while open", func(t *testing.T) {
		var tm tea.Model = open(t, 120, 50)
		tm, _ = tm.Update(tea.WindowSizeMsg{Width: 60, Height: 20})
		fits(t, tm.(Model), "shrunk to 60x20")
	})
}

// Below the minimum the confirm shows no diff rather than a clipped one: it
// says the frame is too short and keeps the footer, since $EDITOR is about to
// show the whole merge anyway.
func TestResolveModalTooShortSaysSo(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var tm tea.Model = New(sampleMemories(), nil, nil, config.Config{})
	tm, _ = tm.Update(tea.WindowSizeMsg{Width: 42, Height: 14})
	m := tm.(Model)
	m.mode = modeResolveConfirm
	m.resolveTmp = "/tmp/merge.md"
	m.setResolveSides([]string{"mine", "a", "b"}, []string{"theirs", "a", "c"}, false)

	if m.resolveDiffRows() != 0 {
		t.Fatalf("resolveDiffRows() = %d at 42x14, want 0 (no room)", m.resolveDiffRows())
	}
	if m.resolveSame != resolveNoRoom {
		t.Errorf("resolveSame = %d, want resolveNoRoom", m.resolveSame)
	}
	if len(m.resolveRows) != 0 {
		t.Errorf("got %d diff rows, want none when there is no room", len(m.resolveRows))
	}
	plain := sgr.ReplaceAllString(m.resolveModal(), "")
	// Checked in two pieces: at this width the sentence wraps mid-phrase, so a
	// contiguous match would be asserting the wrap, not the message.
	if !strings.Contains(plain, "too short to") || !strings.Contains(plain, "preview the diff") {
		t.Errorf("modal doesn't say why there is no diff:\n%s", plain)
	}
	if !strings.Contains(plain, "open $EDITOR") {
		t.Errorf("modal dropped its footer:\n%s", plain)
	}
}

// A resize re-caps the diff to the new frame without re-aligning it: only the
// row budget depends on the frame, and a drag-resize is a stream of these.
func TestResizeRecapsTheDiffWithoutRealigning(t *testing.T) {
	m := ready(t)
	m.mode = modeResolveConfirm
	var yours, theirs []string
	for i := 0; i < 40; i++ {
		yours = append(yours, "mine "+strconv.Itoa(i))
		theirs = append(theirs, "theirs "+strconv.Itoa(i))
	}
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = next.(Model)
	m.mode = modeResolveConfirm
	m.setResolveSides(yours, theirs, false)
	aligned, tall := len(m.resolveAligned), len(m.resolveRows)
	if tall == 0 || aligned == 0 {
		t.Fatalf("setup: aligned=%d rows=%d", aligned, tall)
	}

	next, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 18})
	small := next.(Model)
	if len(small.resolveAligned) != aligned {
		t.Errorf("the alignment was recomputed on resize: %d rows, was %d", len(small.resolveAligned), aligned)
	}
	if len(small.resolveRows) >= tall {
		t.Errorf("a shorter frame kept %d rows (was %d) — the cap did not follow the frame", len(small.resolveRows), tall)
	}
	if got, want := len(small.resolveRows), small.resolveDiffRows(); want > 0 && got > want {
		t.Errorf("rows=%d exceeds the frame's budget of %d", got, want)
	}
	// And growing back restores the taller view from the same alignment.
	next, _ = small.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	if back := next.(Model); len(back.resolveRows) != tall {
		t.Errorf("rows=%d after growing back, want the original %d", len(back.resolveRows), tall)
	}
}
