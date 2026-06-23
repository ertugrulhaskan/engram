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

// clip truncates s to at most w display columns (measuring wide runes
// correctly), appending an ellipsis when it had to cut.
func clip(s string, w int) string {
	if w <= 0 {
		return ""
	}
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
