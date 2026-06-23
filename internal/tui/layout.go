package tui

import (
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/glamour"
)

func (m *Model) resize(w, h int) {
	m.width, m.height = w, h

	// Split into list | divider(1) | preview so the three always sum to width
	// (no horizontal overflow even on narrow terminals).
	m.listW = w * 2 / 5
	if m.listW < 20 {
		m.listW = 20
	}
	if m.listW > w-2 { // keep previewW >= 1
		m.listW = w - 2
	}
	if m.listW < 1 {
		m.listW = 1
	}
	m.previewW = w - m.listW - 1
	if m.previewW < 1 {
		m.previewW = 1
	}

	// Chrome is 4 lines (top bar, sub row, bottom rule, bottom bar) and we leave
	// the terminal's final row unwritten — filling the very last cell makes some
	// terminals scroll the alt-screen buffer on each repaint, which shows up as
	// blank scrollback with the UI pinned to the bottom. That single reservation
	// is the whole scroll fix; no force-clear or frame clamp needed.
	m.panesH = h - 5
	if m.panesH < 6 {
		m.panesH = 6
	}
	m.search.Width = m.listW - 4
	if m.search.Width < 1 {
		m.search.Width = 1
	}
	m.input.Width = m.previewW
	m.previewCache = nil // width changed — rendered bodies must re-wrap

	vpH := m.panesH - 4 // preview meta header is 4 lines
	if vpH < 1 {
		vpH = 1
	}
	innerW := m.previewW - previewPad
	if innerW < 10 {
		innerW = 10
	}
	if !m.ready {
		m.viewport = viewport.New(innerW, vpH)
		m.ready = true
	} else {
		m.viewport.Width = innerW
		m.viewport.Height = vpH
	}
	m.buildRenderer()
	m.ensureVisible()
	m.syncPreview()
}

func (m *Model) buildRenderer() {
	if m.previewW <= 0 {
		return
	}
	wrap := m.previewW - previewPad
	if wrap > maxReadCols {
		wrap = maxReadCols
	}
	if wrap < 1 {
		wrap = 1
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle(m.theme().Glamour),
		glamour.WithWordWrap(wrap),
	)
	if err == nil {
		m.renderer = r
	}
}

func (m Model) listRows() int { return m.panesH - 1 } // last line is the status
