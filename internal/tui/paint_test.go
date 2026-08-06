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

// TestFullFramePainted guards the paint pass end to end: at a normal size,
// every line of the composed frame carries a background (no cell shows the
// terminal's own color through), no line leaves a background open past its
// end, and every line is exactly the terminal width — in all three themes,
// which is the point of painting (Paperback must hold on a dark terminal and
// vice versa).
func TestFullFramePainted(t *testing.T) {
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(old)

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	mems := sampleMemories()
	// Give the first (selected) memory a sync state so the preview's sync strip
	// band is part of the painted frame in every theme.
	mems[0].Shared = memory.EngramMeta{ID: "m-paint", Scope: "team", Project: "global"}
	base := New(mems, samplePlans(), sampleDocs(), config.Config{})
	base.syncStates = map[string]team.SyncState{mems[0].Path: team.StateIncoming}
	base.rebuildRows()

	const w, h = 100, 30
	for themeKey := '1'; themeKey <= '3'; themeKey++ {
		var cur tea.Model = base
		cur, _ = cur.Update(tea.WindowSizeMsg{Width: w, Height: h})
		cur, _ = cur.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{rune(themeKey)}})
		name := cur.(Model).theme().Name

		for i, line := range strings.Split(cur.(Model).View(), "\n") {
			if lw := ansi.StringWidth(line); lw != w {
				t.Errorf("%s: line %d is %d cells wide, want %d", name, i, lw, w)
			}
			seen, open := scanBackground(line)
			if !seen {
				t.Errorf("%s: line %d carries no background — the paint pass must fill every cell", name, i)
			}
			if open {
				t.Errorf("%s: line %d ends with an open background — would bleed into the next row", name, i)
			}
		}
	}
}

// TestPaintLine pins the paint primitive: exact width, background on the
// padding, re-assertion after resets, and a clean degrade when the profile
// strips color.
func TestPaintLine(t *testing.T) {
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(old)

	bg := "#282a36"
	seq := bgSeq(bg)
	if seq == "" {
		t.Fatal("bgSeq returned empty under TrueColor")
	}

	// Pad path: short content gets bg-painted padding and ends closed.
	got := paintLine("hi", 6, bg)
	if lw := ansi.StringWidth(got); lw != 6 {
		t.Errorf("pad: width = %d, want 6", lw)
	}
	if !strings.HasPrefix(got, seq) {
		t.Errorf("pad: line does not open with the background: %q", got)
	}
	if seen, open := scanBackground(got); !seen || open {
		t.Errorf("pad: seen=%v open=%v, want painted and closed", seen, open)
	}

	// Reset re-assertion: a mid-line reset must not punch a hole.
	styled := "\x1b[38;2;200;10;10mred\x1b[0mrest"
	got = paintLine(styled, 10, bg)
	if !strings.Contains(got, "\x1b[0m"+seq) {
		t.Errorf("reset not followed by a bg re-assert: %q", got)
	}

	// Truncate path: overlong content is cut to width and closed.
	got = paintLine(strings.Repeat("x", 12), 5, bg)
	if lw := ansi.StringWidth(got); lw != 5 {
		t.Errorf("truncate: width = %d, want 5", lw)
	}
	if !strings.HasSuffix(got, ansi.ResetStyle) {
		t.Errorf("truncate: line does not end with a reset: %q", got)
	}

	// Ascii profile: byte-clean output, still exact width.
	lipgloss.SetColorProfile(termenv.Ascii)
	got = paintLine("hi", 6, bg)
	if got != "hi    " {
		t.Errorf("ascii: got %q, want %q", got, "hi    ")
	}
	lipgloss.SetColorProfile(termenv.TrueColor)
}

// TestDimFrame pins the scrim: truecolor fg/bg params blend toward the theme
// surface by scrimAmount, bare resets gain the blended default foreground,
// and non-truecolor profiles pass through untouched.
func TestDimFrame(t *testing.T) {
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(old)

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := New(nil, nil, nil, config.Config{Theme: "midnight"})
	th := m.theme()
	baseR, baseG, baseB, _ := hexRGB(th.Bg)
	mix := func(a, b int) int { return int(float64(a)*(1-scrimAmount) + float64(b)*scrimAmount + 0.5) }

	in := "\x1b[38;2;255;255;255mtext\x1b[0mplain"
	got := m.dimFrame(in)
	wantFg := fmt.Sprintf("38;2;%d;%d;%d", mix(255, baseR), mix(255, baseG), mix(255, baseB))
	if !strings.Contains(got, wantFg) {
		t.Errorf("fg not blended toward Bg: got %q, want params %q", got, wantFg)
	}
	// The bare reset keeps resetting, then re-colors default text dimmed.
	if !strings.Contains(got, "\x1b[0;38;2;") {
		t.Errorf("reset did not gain a blended default fg: %q", got)
	}

	// Backgrounds blend too (toward the same surface).
	in = "\x1b[48;2;68;71;90m sel \x1b[0m"
	got = m.dimFrame(in)
	wantBg := fmt.Sprintf("48;2;%d;%d;%d", mix(68, baseR), mix(71, baseG), mix(90, baseB))
	if !strings.Contains(got, wantBg) {
		t.Errorf("bg not blended toward Bg: got %q, want params %q", got, wantBg)
	}

	// Non-truecolor: untouched.
	lipgloss.SetColorProfile(termenv.ANSI256)
	if got := m.dimFrame(in); got != in {
		t.Errorf("non-truecolor frame was rewritten: %q", got)
	}
	lipgloss.SetColorProfile(termenv.TrueColor)
}
