package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ertugrulhaskan/engram/internal/team"
)

// withdrawFinishedMsg reports the outcome of a background Withdraw.
type withdrawFinishedMsg struct {
	pushed bool
	err    error
}

// withdrawCmd runs team.Withdraw off the UI thread (captured git output, like promote).
func (m Model) withdrawCmd(path string) tea.Cmd {
	return func() tea.Msg {
		pushed, err := team.Withdraw(path)
		return withdrawFinishedMsg{pushed: pushed, err: err}
	}
}

// updateWithdrawConfirm drives the confirm: y withdraws, anything else cancels.
func (m Model) updateWithdrawConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		m.mode = modeNormal
		return m, m.withdrawCmd(m.withdrawPath)
	default:
		m.mode = modeNormal
		return m, m.setCancel("cancelled")
	}
}

// withdrawModal confirms removing a shared memory from the team store. It uses the
// accent frame (not danger red) because withdraw is reversible — re-promote
// restores it — unlike a delete.
func (m Model) withdrawModal() string {
	t := m.theme()
	cw := m.boxWidth()
	panel := m.panelBg()
	pst := func(col string) lipgloss.Style { return fg(col).Background(lipgloss.Color(panel)) }

	row := m.palRow(palItem{glyph: "◆", label: m.withdrawTitle, sub: "remove from the team store · resets to personal"}, cw, panel, t.Accent)
	lines := []string{
		padBG(pst(t.Accent).Bold(true).Render(" Withdraw from team?"), cw, panel),
		m.ruleLine(cw),
	}
	// Derive the bleed rows from where the target row lands (mirrors confirmModal).
	bleed := map[int]string{len(lines): t.Accent, len(lines) + 1: t.Accent}
	hint := pst(t.Dim).Render("  press ") + pst(t.Accent).Bold(true).Render("y") + pst(t.Dim).Render(" withdraw     ") +
		pst(t.Fg).Bold(true).Render("n") + pst(t.Dim).Render(" / ") + pst(t.Fg).Bold(true).Render("esc") + pst(t.Dim).Render(" cancel")
	lines = append(lines, row[0], row[1], padBG("", cw, panel), padBG(hint, cw, panel))
	return m.frameLines(lines, cw, t.Accent, bleed)
}
