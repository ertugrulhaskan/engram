package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ertugrulhaskan/engram/internal/team"
)

// promoteFinishedMsg reports the outcome of a background Promote.
type promoteFinishedMsg struct {
	pushed bool
	err    error
}

// promoteCmd runs team.Promote off the UI thread. Promote keeps git output
// captured, so it never disturbs the alt-screen — no tea.ExecProcess takeover is
// needed; a push that needs interactive credentials simply reports pushed=false.
func (m Model) promoteCmd(path, placement string) tea.Cmd {
	return func() tea.Msg {
		pushed, err := team.Promote(path, placement)
		return promoteFinishedMsg{pushed: pushed, err: err}
	}
}

// updatePromoteScope drives the scope picker: ↑/↓ (or j/k/tab) toggles between
// "this project" and "global" — only when a project key exists — enter promotes,
// esc cancels.
func (m Model) updatePromoteScope(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "ctrl+c":
		m.mode = modeNormal
		return m, m.setCancel("cancelled")
	case "up", "k", "down", "j", "tab":
		if m.promoteKey != "" { // a single option (global) isn't navigable
			m.promoteCursor ^= 1
		}
		return m, nil
	case "enter":
		m.mode = modeNormal
		placement := "global"
		if m.promoteKey != "" && m.promoteCursor == 0 {
			placement = m.promoteKey
		}
		if m.scanAction == "off" {
			return m, m.promoteCmd(m.promotePath, placement)
		}
		return m, m.scanCmd(m.promotePath, placement) // scan first; policy applied on the result
	}
	return m, nil
}

// scopeModal renders the promote scope picker in the shared opaque-dialog style:
// a header, the project and global choices (project omitted when the memory's
// project has no git remote), and the action hints.
func (m Model) scopeModal() string {
	t := m.theme()
	cw := m.boxWidth()
	panel := m.panelBg()
	pst := func(col string) lipgloss.Style { return fg(col).Background(lipgloss.Color(panel)) }

	lines := []string{
		padBG(pst(t.Accent).Bold(true).Render(" Promote “"+clip(m.promoteTitle, cw-14)+"”"), cw, panel),
		m.ruleLine(cw),
	}
	bleed := map[int]string{}

	addRow := func(label, sub string, selected bool) {
		selBg := ""
		if selected {
			selBg = t.Accent
		}
		row := m.palRow(palItem{glyph: "◆", label: label, sub: sub}, cw, panel, selBg)
		if selBg != "" {
			bleed[len(lines)] = selBg
			bleed[len(lines)+1] = selBg
		}
		lines = append(lines, row...)
	}

	if m.promoteKey != "" {
		addRow("This project", m.promoteKey, m.promoteCursor == 0)
		addRow("Global", "shared across all projects", m.promoteCursor == 1)
	} else {
		lines = append(lines, padBG(pst(t.Dim).Render("  this project has no git remote — promoting globally"), cw, panel))
		addRow("Global", "shared across all projects", true)
	}

	hint := pst(t.Dim).Render("  ↑↓ choose   ") + pst(t.Accent).Bold(true).Render("↵") + pst(t.Dim).Render(" promote   ") +
		pst(t.Fg).Bold(true).Render("esc") + pst(t.Dim).Render(" cancel")
	lines = append(lines, padBG("", cw, panel), padBG(hint, cw, panel))
	return m.frameLines(lines, cw, t.Accent, bleed)
}
