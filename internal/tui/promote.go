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
	count    int // memories promoted in one commit; 0 or 1 = the single-memory wording
	skipped  int // flagged memories the user declined during the scan walk
	overrode int // flagged memories the user included anyway — never left unsaid
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
		// Disarm the batch. A cancelled batch left in the model would be picked up
		// by the next promote — including a single-memory one, which reads
		// m.batchItems first — and promote memories the user did not select.
		m.mode = modeNormal
		m.batchItems = nil
		return m, m.setCancel("cancelled")
	case "up", "k", "down", "j", "tab":
		navigable := m.promoteKey != ""
		if len(m.batchItems) > 0 {
			navigable = anyKeyed(m.batchItems)
		}
		if navigable { // a single option (global) isn't navigable
			m.promoteCursor ^= 1
		}
		return m, nil
	case "enter":
		m.mode = modeNormal
		if len(m.batchItems) > 0 {
			items := resolvePlacements(m.batchItems, m.promoteCursor == 0)
			m.batchItems = items
			if m.scanAction == "off" {
				return m, m.promoteBatchCmd(items, 0, 0)
			}
			return m, m.batchScanCmd(items) // every memory is scanned, not just the first
		}
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

// noKeyLine explains a promote that can only go global: lead states the lack
// of a key ("this project has no key to promote under"), and the reason that
// follows depends on what kept the key away — with no remote, >alias would
// give one; when git could not say, no alias would be consulted, so that hint
// would mislead.
func noKeyLine(lead string, state team.RemoteState) string {
	switch state {
	case team.RemoteUnknown:
		return lead + " — git couldn't tell whether it has a remote — promoting globally"
	case team.RemoteGone:
		return lead + " — its directory is gone — promoting globally"
	}
	return lead + " — promoting globally (>alias <name> keys a project that has no git remote)"
}

// scopeModal renders the promote scope picker in the shared dialog anatomy
// (Warn — promoting publishes): scope is the question, so the two scopes are
// the selectable rows, then the honest mechanics line. The project row is
// omitted when the memory's project has no git remote.
func (m Model) scopeModal() string {
	t := m.theme()
	cw := m.boxWidth()
	panel := m.panelBg()

	title := "promote “" + clip(m.promoteTitle, cw-24) + "” — pick a scope"
	if len(m.batchItems) > 0 {
		title = batchHeader(len(m.batchItems))
	}
	lines := m.dlgHeader(cw, "→", title, t.Warn)
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

	switch {
	case len(m.batchItems) > 0 && anyKeyed(m.batchItems):
		addRow("their own projects", batchScopeSub(m.batchItems), m.promoteCursor == 0)
		addRow("global", "every project you work in", m.promoteCursor == 1)
	case len(m.batchItems) > 0:
		lines = append(lines, m.dlgText(cw, noKeyLine("none of these projects has a key to promote under", team.RemoteNone), t.Dim)...)
		addRow("global", "every project you work in", true)
	case m.promoteKey != "":
		keyed := "keyed by " + m.promoteKey
		if team.IsAliasKey(m.promoteKey) {
			keyed += " (your alias)"
		}
		addRow("this project", keyed, m.promoteCursor == 0)
		addRow("global", "every project you work in", m.promoteCursor == 1)
	default:
		lines = append(lines, m.dlgText(cw, noKeyLine("this project has no key to promote under", m.promoteState), t.Dim)...)
		addRow("global", "every project you work in", true)
	}
	lines = append(lines, padBG("", cw, panel))
	mechanics := "engram stamps an engram: frontmatter block and pushes."
	if len(m.batchItems) > 1 {
		mechanics = "engram stamps each memory and pushes all of them as one commit."
	}
	lines = append(lines, m.dlgText(cw, mechanics, t.Dim)...)
	lines = append(lines, padBG("", cw, panel))

	bleed[len(lines)] = t.Bg2
	lines = append(lines, m.dlgFooter(cw, t.Warn, []dialogAction{{"esc cancel", false}, {"↵ promote", true}}))
	return m.frameLines(lines, cw, t.Warn, bleed)
}
