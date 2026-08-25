package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"

	"github.com/ertugrulhaskan/engram/internal/memory"
)

func typeLabel(t memory.Type) string {
	switch t {
	case memory.TypeUser:
		return "user"
	case memory.TypeFeedback:
		return "feedback"
	case memory.TypeProject:
		return "project"
	case memory.TypeReference:
		return "reference"
	default:
		return "other"
	}
}

// typeName is the badge label for a type.
func typeName(t memory.Type) string {
	if t == memory.TypeUnknown || t == "" {
		return "other"
	}
	return string(t)
}

// --- text helpers ---

func fg(c string) lipgloss.Style  { return lipgloss.NewStyle().Foreground(lipgloss.Color(c)) }
func fgb(c string) lipgloss.Style { return fg(c).Bold(true) }

// onbg styles text with an explicit surface color, for chrome that sits on a
// painted background other than the bars' Bg2 (e.g. the source strip on Bg).
func onbg(c, bg string) lipgloss.Style { return fg(c).Background(lipgloss.Color(bg)) }

func humanizeSince(t time.Time) string {
	if t.IsZero() {
		return "unknown"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 48*time.Hour:
		return "yesterday"
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	case d < 28*24*time.Hour:
		return fmt.Sprintf("%dw ago", int(d.Hours()/(24*7)))
	default:
		return t.Format("Jan 2, 2006")
	}
}

// stripFirstHeading removes a leading "# ..." line (and a following blank) so
// the preview's own title isn't duplicated by the rendered body.
func stripFirstHeading(body string) string {
	lines := strings.Split(body, "\n")
	for i, ln := range lines {
		s := strings.TrimSpace(ln)
		if s == "" {
			continue
		}
		if strings.HasPrefix(s, "# ") {
			rest := lines[i+1:]
			for len(rest) > 0 && strings.TrimSpace(rest[0]) == "" {
				rest = rest[1:]
			}
			return strings.Join(rest, "\n")
		}
		break
	}
	return body
}

func clampIdx(i, n int) int {
	if i < 0 {
		return 0
	}
	if i >= n {
		if n == 0 {
			return 0
		}
		return n - 1
	}
	return i
}

// isTerminalControl reports whether r is a character a terminal acts on rather
// than prints: the C0 range, DEL, and C1. Callers decide which of \n and \t to
// keep before consulting it.
func isTerminalControl(r rune) bool {
	return r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f)
}

// sanitizeLine drops the control characters from a one-line string, turning
// tabs and newlines into a space so the width math below stays honest. A
// newline becomes a space rather than vanishing because multi-line text does
// reach one-line sinks — a git error carrying the subprocess's CombinedOutput
// is the common case — and dropping it would run the last word of one line
// into the first word of the next.
//
// engram renders files it does not own. /files lists instruction files from any
// project under scanRoots — including repositories the user has only cloned —
// and a rule file's frontmatter reaches the list as a Detail line. Neither
// glamour nor lipgloss strips an escape sequence embedded in the text they are
// handed, so without this a `globs:` value carrying an OSC sequence would reach
// the terminal verbatim and rewrite the window title (or, where OSC 52 is
// permitted, the clipboard). Sanitizing here rather than at each call site is
// deliberate: every one-line sink funnels through one of three chokepoints —
// clip for metadata, wrapPlain for dialog bodies, flashStatus for the status
// bar — so the guard covers the whole class instead of a list of call sites.
// Styling is always applied *after* each of them, so no legitimate ANSI is
// ever passed in for this to eat.
//
// Bidi overrides (U+202A-U+202E) are deliberately left alone: they are valid in
// right-to-left text, and visual deception is a different problem from terminal
// control.
func sanitizeLine(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\t' || r == '\n' {
			return ' '
		}
		if isTerminalControl(r) {
			return -1
		}
		return r
	}, s)
}

// sanitizeBody is sanitizeLine for multi-line markdown on its way *into* the
// renderer: newlines and tabs survive because markdown uses both structurally.
// It must run before glamour, never after — glamour's own output is ANSI by
// design.
func sanitizeBody(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return r
		}
		if isTerminalControl(r) {
			return -1
		}
		return r
	}, s)
}

// clip truncates s to at most w display columns (measuring wide runes
// correctly), appending an ellipsis when it had to cut. Control characters are
// stripped first — see sanitizeLine — which also keeps the width measurement
// honest, since an escape sequence would otherwise be counted as printable
// columns it never occupies.
func clip(s string, w int) string {
	if w <= 0 {
		return ""
	}
	s = sanitizeLine(s)
	if runewidth.StringWidth(s) <= w {
		return s
	}
	return runewidth.Truncate(s, w, "…")
}

// padRight clips s to w display columns then right-pads to exactly w.
func padRight(s string, w int) string {
	if w <= 0 {
		return ""
	}
	s = clip(s, w)
	return s + spaces(w-runewidth.StringWidth(s))
}

// padLeft clips s to w display columns then left-pads to exactly w.
func padLeft(s string, w int) string {
	if w <= 0 {
		return ""
	}
	s = clip(s, w)
	return spaces(w-runewidth.StringWidth(s)) + s
}

// padTo right-pads a possibly-styled string to width w (display columns).
func padTo(s string, w int) string {
	gap := w - lipgloss.Width(s)
	if gap < 0 {
		return s
	}
	return s + spaces(gap)
}

func spaces(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.Repeat(" ", n)
}
