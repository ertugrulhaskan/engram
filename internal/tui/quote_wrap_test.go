package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/ertugrulhaskan/engram/internal/config"
)

// quoteBarStyles records which Glamour styles draw a │ bar beside a blockquote.
// It is the expectation under test, so it is written down rather than read off
// the output: a style that silently stopped drawing its bar must fail, not pass
// as "a theme without one". A theme whose style is missing here fails the test,
// so adding a theme means deciding this.
var quoteBarStyles = map[string]bool{"dark": true, "light": true, "dracula": false}

// A wrapped blockquote must keep its │ bar on every line. Glamour 1.0 wraps
// quote text without counting the bar's own cells and then wraps again, so one
// word falls onto a bar-less line at about a third of all widths (29 such lines
// across wraps 30-88 for the first quote below; 0 on Glamour 0.8). v0.6.0
// shipped that; v0.6.1 went back to 0.8. Dependabot will offer 1.0 again and
// the rest of the suite passes on it, so this test is what makes that bump
// fail CI.
//
// It drives the real buildRenderer in every theme, at every wrap width from
// the narrowest that can hold the quote's longest word beside the bar up to
// maxReadCols. The quotes are plain prose on purpose: Glamour 0.8 itself
// mishandles links in a wrapped quote, so a link sample would fail on the
// version engram ships.
func TestBlockquoteKeepsBarWhenWrapped(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	quotes := map[string]string{
		"one paragraph": "> Never push to main. Work on a branch and open a pull request so the " +
			"maintainer can review and merge it themselves, and never force-push over shared history.\n",
		"two paragraphs": "> First paragraph of a quote that is long enough to wrap more than once " +
			"at the narrower widths a preview pane gets.\n>\n> Second paragraph, also long " +
			"enough to wrap at least once at a narrow width like these.\n",
		"short words": "> quote aaaa bbbb cccc dddd eeee ffff gggg hhhh iiii jjjj kkkk llll mmmm " +
			"nnnn oooo pppp qqqq rrrr ssss tttt uuuu vvvv wwww xxxx\n",
	}

	base := New(sampleMemories(), nil, nil, config.Config{})
	for name, md := range quotes {
		longest := 0
		for _, w := range strings.Fields(strings.ReplaceAll(md, ">", " ")) {
			longest = max(longest, lipgloss.Width(w))
		}
		minWrap := longest + lipgloss.Width("│ ") + 2*previewPad // word + bar + document margins

		for ti, th := range themes {
			wantBar, known := quoteBarStyles[th.Glamour]
			if !known {
				t.Fatalf("theme %s uses Glamour style %q: add it to quoteBarStyles", th.Name, th.Glamour)
			}
			for wrap := minWrap; wrap <= maxReadCols; wrap++ {
				m := base
				m.themeIdx = ti
				m.previewW = wrap + 2*previewPad
				m.renderer = nil
				m.buildRenderer()
				if m.renderer == nil {
					t.Fatalf("%s: no renderer at wrap %d", th.Name, wrap)
				}
				out, err := m.renderer.Render(md)
				if err != nil {
					t.Fatalf("%s %s wrap %d: %v", name, th.Name, wrap, err)
				}
				// Every quote line must start at the same indent and, where the
				// style draws one, carry the bar. Midnight's dracula style draws no
				// bar and wraps correctly even on Glamour 1.0 (no extra cells to
				// miscount); the bug shows on Paperback and CRT. The indent check
				// still guards all three, since a spilled word starts further left.
				indent := -1
				for _, ln := range strings.Split(ansi.Strip(out), "\n") {
					txt := strings.TrimSpace(ln)
					if txt == "" {
						continue
					}
					in := len(ln) - len(strings.TrimLeft(ln, " "))
					if indent < 0 {
						indent = in
					}
					if in != indent {
						t.Errorf("%s, %s wrap %d: quote line at indent %d, want %d: %q", name, th.Name, wrap, in, indent, txt)
					}
					if bar := strings.HasPrefix(txt, "│"); bar != wantBar {
						t.Errorf("%s, %s wrap %d: bar=%v, want %v: %q", name, th.Name, wrap, bar, wantBar, txt)
					}
				}
			}
		}
	}
}
