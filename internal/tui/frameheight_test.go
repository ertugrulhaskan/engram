package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ertugrulhaskan/engram/internal/config"
	"github.com/ertugrulhaskan/engram/internal/memory"
)

// stressMemories prepends a memory with a very long title and long, wrappable
// body so the preview pane is forced to wrap heavily — the case that used to
// push the frame past the terminal height on narrow terminals.
func stressMemories() []memory.Memory {
	long := "This is a deliberately long paragraph that will wrap several times inside a narrow preview pane and produce many rendered lines of body content. "
	body := "# Really long memory title that itself may need to be clipped in the preview\n\n" +
		strings.Repeat(long, 6) + "\n\n- " + strings.Repeat("bullet text that also wraps around a few times ", 4) + "\n"
	mm := memory.Memory{
		Title:    "Really long memory title that itself may need to be clipped in the preview",
		Body:     body,
		Type:     memory.TypeProject,
		Path:     "/Users/me/.claude/projects/-x/memory/long.md",
		Modified: time.Now(),
		Project:  memory.Project{Name: "engram", MemoryDir: "/Users/me/.claude/projects/-x/memory"},
	}
	return append([]memory.Memory{mm}, sampleMemories()...)
}

// TestFrameNeverExceedsTerminal guards the invariant behind the "ghost/multiple
// selected rows" bug: View() must never emit more lines than the terminal has
// rows (frame height <= H-1, the last row is reserved) nor a line wider than the
// terminal. A taller-than-terminal frame scrolls the alt-screen and desyncs
// Bubble Tea's line-diff renderer, leaving a trail of highlighted rows until the
// next full repaint (a resize).
func TestFrameNeverExceedsTerminal(t *testing.T) {
	// No SetColorProfile: line count and lipgloss.Width are color-independent, and
	// a global profile change would leak into sibling tests in this package.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	base := New(stressMemories(), samplePlans(), sampleDocs(), config.Config{})

	// Include very short terminals (h <= 11): panesH is floored above h-5 there,
	// so without the height clamp the frame would overflow at any width — the
	// path a resize drag passes through, seeding ghost rows at the final size.
	for _, h := range []int{6, 8, 10, 11, 12, 14, 20, 22, 24, 26, 30, 40} {
		for _, w := range []int{24, 26, 28, 30, 33, 36, 40, 50, 60, 80, 100, 140} {
			var cur tea.Model = base
			cur, _ = cur.Update(tea.WindowSizeMsg{Width: w, Height: h})
			m := cur.(Model)
			m.cursor = m.firstMemRow() // select the long memory: worst-case preview
			m.syncPreview()
			frame := m.View()
			got := strings.Count(frame, "\n") + 1
			maxw := 0
			for _, ln := range strings.Split(frame, "\n") {
				if lw := lipgloss.Width(ln); lw > maxw {
					maxw = lw
				}
			}
			if got > h-1 {
				t.Errorf("W=%d H=%d: frame is %d lines, want <= %d (overflow scrolls the alt-screen)", w, h, got, h-1)
			}
			if maxw > w {
				t.Errorf("W=%d H=%d: widest line is %d cells, want <= %d", w, h, maxw, w)
			}
		}
	}
}
