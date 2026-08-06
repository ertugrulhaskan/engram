package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/ertugrulhaskan/engram/internal/team"
)

// promoteFinishedMsg reports the outcome of a background Promote. override
// travels with it so the toast can say the secret-scan block was overridden.
type promoteFinishedMsg struct {
	pushed   bool
	override bool
	err      error
}

// promoteCmd runs team.Promote off the UI thread. Promote keeps git output
// captured, so it never disturbs the alt-screen — no tea.ExecProcess takeover is
// needed; a push that needs interactive credentials simply reports pushed=false.
func (m Model) promoteCmd(path, placement string, override bool) tea.Cmd {
	return func() tea.Msg {
		pushed, err := team.Promote(path, placement)
		return promoteFinishedMsg{pushed: pushed, override: override, err: err}
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
			return m, m.promoteCmd(m.promotePath, placement, false)
		}
		return m, m.scanCmd(m.promotePath, placement) // scan first; policy applied on the result
	}
	return m, nil
}

// scopeModal renders the promote scope picker in the shared dialog anatomy
// (Warn — promoting publishes): scope is the question, so the two scopes are
// the selectable rows, then the honest mechanics line. The project row is
// omitted when the memory's project has no git remote.
func (m Model) scopeModal() string {
	t := m.theme()
	cw := m.boxWidth()
	panel := m.panelBg()

	lines := m.dlgHeader(cw, "→", "promote “"+clip(m.promoteTitle, cw-24)+"” — pick a scope", t.Warn)
	bleed := map[int]string{}

	addRow := func(label, sub string, selected bool) {
		txt := clip(label+" — "+sub, cw-4)
		if selected {
			bleed[len(lines)] = t.Sel
			row := onbg(t.Warn, t.Sel).Bold(true).Render("  › ") + onbg(t.Fg, t.Sel).Bold(true).Render(txt)
			lines = append(lines, padBG(row, cw, t.Sel))
			return
		}
		lines = append(lines, padBG(onbg(t.Dim, panel).Render("    "+txt), cw, panel))
	}

	if m.promoteKey != "" {
		addRow("this project", "keyed by "+m.promoteKey, m.promoteCursor == 0)
		addRow("global", "every project you work in", m.promoteCursor == 1)
	} else {
		lines = append(lines, m.dlgText(cw, "this project has no git remote — promoting globally", t.Dim)...)
		addRow("global", "every project you work in", true)
	}
	lines = append(lines, padBG("", cw, panel))
	lines = append(lines, m.dlgText(cw, "engram stamps an engram: frontmatter block and pushes.", t.Dim)...)
	lines = append(lines, padBG("", cw, panel))

	bleed[len(lines)] = t.Bg2
	lines = append(lines, m.dlgFooter(cw, t.Warn, []dialogAction{{"esc cancel", false}, {"↵ promote", true}}))
	return m.frameLines(lines, cw, t.Warn, bleed)
}
