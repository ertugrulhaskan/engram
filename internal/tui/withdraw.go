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
func (m Model) withdrawModal() string {
	t := m.theme()
	return m.dialog("↩", "withdraw this shared memory?", t.Warn,
		[]string{
			m.withdrawTitle,
			"Removed from the store, reset to personal, and a tombstone removes it from teammates on their next pull.",
			"Promoting again puts it back.",
		},
		[]dialogAction{{"n cancel", false}, {"y withdraw", true}})
}
