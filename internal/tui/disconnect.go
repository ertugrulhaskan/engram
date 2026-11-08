package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/ertugrulhaskan/engram/internal/team"
)

// disconnectFinishedMsg reports the outcome of a background team.Disconnect.
type disconnectFinishedMsg struct {
	kept int
	err  error
}

// teamMemoryPaths is every loaded memory whose engram block says team: the
// ones a disconnect keeps as personal.
func (m Model) teamMemoryPaths() []string {
	var out []string
	for _, mm := range m.memories {
		if mm.Shared.Scope == "team" {
			out = append(out, mm.Path)
		}
	}
	return out
}

// actionDisconnect opens the disconnect confirm. It is store-wide, so it works
// from any source; whether the clone holds unpushed work is checked by
// team.Disconnect itself, which refuses with the reason.
func (m Model) actionDisconnect() (tea.Model, tea.Cmd) {
	if cmd := m.teamBlocked(); cmd != nil { // also turns away a second >disconnect
		return m, cmd
	}
	if !team.IsInitialized() {
		return m, m.setStatus("no team store connected")
	}
	m.disconnectPaths = m.teamMemoryPaths()
	m.mode = modeDisconnectConfirm
	return m, nil
}

// disconnectCmd runs team.Disconnect off the UI thread.
func (m Model) disconnectCmd(paths []string) tea.Cmd {
	return func() tea.Msg {
		kept, err := team.Disconnect(paths)
		return disconnectFinishedMsg{kept: kept, err: err}
	}
}

// updateDisconnectConfirm drives the confirm: y disconnects, anything else cancels.
func (m Model) updateDisconnectConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.mode = modeNormal
	switch msg.String() {
	case "y", "Y":
		m.disconnecting = true
		return m, m.disconnectCmd(m.disconnectPaths)
	default:
		return m, m.setCancel("cancelled")
	}
}

// disconnectSummary is the status line after a disconnect.
func disconnectSummary(kept int) string {
	switch kept {
	case 0:
		return "team store disconnected"
	case 1:
		return "team store disconnected: 1 shared memory kept as personal"
	}
	return fmt.Sprintf("team store disconnected: %d shared memories kept as personal", kept)
}

// disconnectModal confirms disconnecting the team store. Warn, like withdraw:
// nothing is deleted, and >init connects a store again.
func (m Model) disconnectModal() string {
	t := m.theme()
	cw := m.boxWidth()
	panel := m.panelBg()

	var kept string
	switch n := len(m.disconnectPaths); n {
	case 0:
		kept = "No shared memories on this machine to change."
	case 1:
		kept = "1 shared memory stays on this machine as a personal one."
	default:
		kept = fmt.Sprintf("%d shared memories stay on this machine as personal ones.", n)
	}
	lines := m.dlgHeader(cw, "←", "disconnect the team store?", t.Warn)
	if m.storeURL != "" {
		lines = append(lines, m.dlgText(cw, tailFit(m.storeURL, cw-4), t.Fg)...)
	}
	lines = append(lines, m.dlgText(cw, kept, t.Dim)...)
	lines = append(lines, m.dlgText(cw, "Nothing is deleted from them or from the remote. Run >init <git-url> to connect a store again.", t.Dim)...)
	lines = append(lines, padBG("", cw, panel))
	bleed := map[int]string{len(lines): t.Bg2}
	lines = append(lines, m.dlgFooter(cw, t.Warn, []dialogAction{{"n cancel", false}, {"y disconnect", true}}))
	return m.frameLines(lines, cw, t.Warn, bleed)
}

// tailFit keeps s within w cells by cutting from the left with "…". A store
// URL's identifying part is its end (the repo name), so a long one gives up its
// scheme and host rather than its name; dlgText would cut the end instead.
func tailFit(s string, w int) string {
	sw := lipgloss.Width(s)
	if sw <= w || w < 1 {
		return s
	}
	return ansi.TruncateLeft(s, sw-w+lipgloss.Width("…"), "…")
}
