package tui

import (
	tea "github.com/charmbracelet/bubbletea"

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

// withdrawModal confirms removing a shared memory from the team store, in the
// shared dialog anatomy. Warn (not Danger): withdrawing is reversible —
// promoting again puts it back — unlike a delete.
//
// Assembled from the dialog pieces rather than dialog(), because an unverifiable
// owner check has to read as a caution (Warn) — dialog() puts every body line
// after the first in Dim, which would bury it.
func (m Model) withdrawModal() string {
	t := m.theme()
	cw := m.boxWidth()
	panel := m.panelBg()

	lines := m.dlgHeader(cw, "↩", "withdraw this shared memory?", t.Warn)
	lines = append(lines, m.dlgText(cw, m.withdrawTitle, t.Fg)...)
	lines = append(lines, m.dlgText(cw, "Removed from the store, reset to personal, and a tombstone removes it from teammates on their next pull.", t.Dim)...)
	lines = append(lines, m.dlgText(cw, "Promoting again puts it back.", t.Dim)...)
	if cause := ownerUnverified(m.withdrawOwner); cause != "" {
		lines = append(lines, padBG("", cw, panel))
		lines = append(lines, m.dlgText(cw, cause, t.Warn)...)
		lines = append(lines, m.dlgText(cw, "Withdrawing is still allowed — this check catches accidents, not attacks.", t.Dim)...)
	}
	lines = append(lines, padBG("", cw, panel))
	bleed := map[int]string{len(lines): t.Bg2}
	lines = append(lines, m.dlgFooter(cw, t.Warn, []dialogAction{{"n cancel", false}, {"y withdraw", true}}))
	return m.frameLines(lines, cw, t.Warn, bleed)
}

// ownerUnverified names why withdraw's owner guardrail could not run, or "" when
// it could. The two causes are reported separately on purpose: they have the same
// outcome but different fixes — one is the memory's history, the other is a
// one-line git config away.
func ownerUnverified(o team.OwnerStatus) string {
	switch {
	case o.Verifiable():
		return ""
	case o.Owner == "" && o.Me == "":
		return "Ownership unverified: this memory records no owner, and no git email is set on this machine."
	case o.Owner == "":
		return "Ownership unverified: this memory records no owner, so there is nothing to check against."
	default:
		return "Ownership unverified: no git email is set on this machine (git config user.email), so engram cannot confirm this is yours."
	}
}
