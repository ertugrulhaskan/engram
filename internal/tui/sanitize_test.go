package tui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ertugrulhaskan/engram/internal/config"
	"github.com/ertugrulhaskan/engram/internal/memory"
)

// osc is an OSC 0 sequence — "set the window title". It stands in for the whole
// class here because it is unmistakable in a rendered frame and, unlike an SGR
// colour code, nothing engram draws would ever emit it legitimately.
const osc = "\x1b]0;PWNED\x07"

// TestClipStripsTerminalControls pins the single-line half of the guard. clip is
// the chokepoint every one-line string in the UI passes through, so stripping
// here covers titles, paths, badges and a rule file's Detail line at once.
func TestClipStripsTerminalControls(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"osc sequence", "applies to " + osc + "*.ts", "applies to ]0;PWNED*.ts"},
		{"bare escape", "a\x1bb", "ab"},
		{"csi sequence", "a\x1b[31mb", "a[31mb"},
		{"carriage return", "a\rb", "ab"},
		{"backspace", "a\bb", "ab"},
		{"del", "a\x7fb", "ab"},
		{"c1 range (CSI)", "a\u009bb", "ab"},
		{"tab becomes a space", "a\tb", "a b"},
		{"newline becomes a space", "a\nb", "a b"},
		{"plain text untouched", "applies to src/**/*.ts", "applies to src/**/*.ts"},
		{"wide runes untouched", "日本語", "日本語"},
	}
	for _, c := range cases {
		if got := clip(c.in, 80); got != c.want {
			t.Errorf("%s: clip(%q) = %q, want %q", c.name, c.in, got, c.want)
		}
	}
}

// TestClipWidthIgnoresStrippedControls guards the second-order bug: an escape
// sequence occupies zero display columns, so counting its bytes as printable
// width would truncate a line that actually fits. Stripping before measuring is
// what keeps the width honest.
func TestClipWidthIgnoresStrippedControls(t *testing.T) {
	// 8 printable columns, plus an escape that occupies none.
	in := "abcd" + osc + "efgh"
	if got := clip(in, 8); got != "abcd]0;PWNEDefgh" {
		// The defanged text is longer than 8 columns, so it *should* truncate —
		// what must not happen is the raw escape surviving.
		if strings.Contains(got, "\x1b") {
			t.Errorf("clip leaked an escape while truncating: %q", got)
		}
	}
	// Without the escape the string fits exactly and must come back whole.
	if got := clip("abcdefgh", 8); got != "abcdefgh" {
		t.Errorf("clip truncated a string that fits: %q", got)
	}
}

// TestSanitizeBodyKeepsMarkdownStructure pins the multi-line half. The body is
// markdown on its way *into* glamour, so newlines and tabs have to survive —
// they carry paragraph and code-block structure — while everything a terminal
// would act on is dropped.
func TestSanitizeBodyKeepsMarkdownStructure(t *testing.T) {
	in := "# Heading\n\n\tindented code\n\nbody " + osc + " text\r\n"
	got := sanitizeBody(in)
	if strings.Contains(got, "\x1b") || strings.Contains(got, "\x07") {
		t.Errorf("sanitizeBody left a control character: %q", got)
	}
	if !strings.Contains(got, "\n\n") {
		t.Errorf("sanitizeBody destroyed paragraph breaks: %q", got)
	}
	if !strings.Contains(got, "\tindented code") {
		t.Errorf("sanitizeBody dropped a tab that carries code-block structure: %q", got)
	}
	if strings.Contains(got, "\r") {
		t.Errorf("sanitizeBody kept a carriage return: %q", got)
	}
}

// TestPreviewDefangsTerminalEscapes drives the whole class end to end, which is
// the only version of this test that proves the guard is wired in rather than
// merely present. A rule file from a repository the user has only *cloned* can
// carry an escape sequence in both its frontmatter scoping and its body; neither
// may reach the terminal. Both libraries in the path pass embedded escapes
// through untouched — verified, not assumed — so the guard has to be engram's.
func TestPreviewDefangsTerminalEscapes(t *testing.T) {
	projDir := "/Users/me/code/cloned"
	docs := []memory.DocFile{
		{
			Path:  projDir + "/.cursor/rules/evil.mdc",
			Title: "evil.mdc" + osc,
			Body:  "Body " + osc + " text.\n",
			// The Detail line is the newest path and the one that bypasses
			// glamour entirely, going straight through clip to the terminal.
			Detail:      "applies to " + osc + "*.ts",
			Kind:        memory.DocRules,
			Provider:    memory.ProviderCursor,
			Scope:       "cloned",
			ProjectName: "cloned",
			ProjectDir:  projDir,
		},
	}
	var tm tea.Model = New(nil, nil, docs, config.Config{})
	tm, _ = tm.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	tm = switchVia(tm, "/files")

	// Deliberately not ansi.Strip: that would remove the very sequence under
	// test and turn this into a test that can never fail.
	frame := tm.(Model).View()
	if strings.Contains(frame, "\x1b]") {
		t.Error("an OSC escape from the rule file reached the rendered frame")
	}
	if strings.Contains(frame, "\x07") {
		t.Error("a BEL from the rule file reached the rendered frame")
	}
	// The defanged text should still be visible — the guard neuters the
	// sequence, it does not silently blank the row.
	if !strings.Contains(frame, "PWNED") {
		t.Error("the rule file's text vanished entirely instead of being defanged")
	}
}

// TestDeleteConfirmDefangsTerminalEscapes drives the dialog body end to end.
// The body is a one-line sink clip never covered: wrapPlain clips only a word
// wider than the line, an escape sequence carries no whitespace to split on,
// and its ESC and BEL bytes measure zero columns — so a short escape-bearing
// title passed through byte for byte. A pulled memory's title reaches this
// dialog, which is what puts the team store on the far end of it.
func TestDeleteConfirmDefangsTerminalEscapes(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	mems := sampleMemories()
	mems[0].Title = "notes" + osc
	var tm tea.Model = New(mems, samplePlans(), nil, config.Config{})
	tm, _ = tm.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	tm, _ = tm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	if got := tm.(Model).mode; got != modeConfirm {
		t.Fatalf("mode = %v, want modeConfirm — the fixture never reached the dialog body", got)
	}

	// Deliberately not ansi.Strip: that would remove the very sequence under
	// test and turn this into a test that can never fail.
	frame := tm.(Model).View()
	if strings.Contains(frame, "\x1b]") {
		t.Error("an OSC escape from a memory title reached the delete confirm")
	}
	if strings.Contains(frame, "\x07") {
		t.Error("a BEL from a memory title reached the delete confirm")
	}
	if !strings.Contains(frame, "PWNED") {
		t.Error("the title vanished entirely instead of being defanged")
	}
}

// The status bar is the third one-line sink. Most of what lands there is an
// err.Error(), and some of those carry text engram does not control — a store
// memory's owner field, or git's own captured output.
func TestStatusBarDefangsTerminalEscapes(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var tm tea.Model = ready(t)
	tm, _ = tm.Update(promoteFinishedMsg{err: errors.New("only " + osc + "@e.com may withdraw")})
	got := tm.(Model)
	if strings.ContainsAny(got.status, "\x1b\x07") {
		t.Errorf("status kept a control character: %q", got.status)
	}
	frame := got.View()
	if strings.Contains(frame, "\x1b]") || strings.Contains(frame, "\x07") {
		t.Error("an escape from an error message reached the rendered frame")
	}
	if !strings.Contains(frame, "PWNED") {
		t.Error("the error text vanished instead of being defanged")
	}
}

// A newline maps to a space rather than being dropped: multi-line errors do
// reach this one-line sink — git's CombinedOutput is the common source — and
// deleting the newline would run the last word of one line into the first of
// the next.
func TestStatusBarNewlineBecomesSpace(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var tm tea.Model = ready(t)
	tm, _ = tm.Update(promoteFinishedMsg{err: errors.New("fatal: refusing\nhint: try again")})
	if got := tm.(Model).status; !strings.Contains(got, "refusing hint:") {
		t.Errorf("status = %q, want the newline rendered as a space", got)
	}
}

// palLine ran its sub text through clip -- and so through sanitizeLine -- only
// when the text needed truncating, so a sub short enough to fit reached
// lipgloss.Render raw. A Jump-to row's sub is a project name read off disk,
// which engram does not own, so that was a real gap in one of style.go's three
// chokepoints, exactly the width of "it fits".
func TestPaletteSubIsSanitizedEvenWhenItFits(t *testing.T) {
	m := ready(t)
	m.mode = modePalette
	m.palRows = []palItem{{glyph: "·", label: "a memory", sub: "proj" + osc}}
	m.palCursor = 0
	out := m.View()
	if strings.Contains(out, "\x1b]0;") {
		t.Error("a short palette sub reached the renderer with its escape sequence intact")
	}
	// The legitimate text survives; only the control bytes go.
	if !strings.Contains(out, "proj") {
		t.Error("sanitizing removed the text too")
	}
	// And the long form, which clip already covered, still does.
	m.palRows = []palItem{{glyph: "·", label: "a memory", sub: strings.Repeat("x", 200) + osc}}
	if strings.Contains(m.View(), "\x1b]0;") {
		t.Error("a clipped palette sub leaked its escape sequence")
	}
}
