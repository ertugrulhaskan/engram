package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// clearStatusMsg auto-dismisses a transient status after its timer elapses.
type clearStatusMsg struct{ seq int }

// statusKind is the severity of a transient footer message; it selects the
// color the message renders in (see statusStyle).
type statusKind int

const (
	statusInfo   statusKind = iota // neutral, theme foreground on the bar
	statusDanger                   // warnings & destructive results: white on red
	statusCancel                   // aborted actions: dark brown on emerald
)

// setStatus shows a neutral transient footer message. setDanger and setCancel
// are the colored variants; all three share flashStatus.
func (m *Model) setStatus(s string) tea.Cmd { return m.flashStatus(s, statusInfo) }
func (m *Model) setDanger(s string) tea.Cmd { return m.flashStatus(s, statusDanger) }
func (m *Model) setCancel(s string) tea.Cmd { return m.flashStatus(s, statusCancel) }

// flashStatus stores the message and its kind, then returns a command that
// clears it after a short delay (unless a newer status replaces it first).
func (m *Model) flashStatus(s string, k statusKind) tea.Cmd {
	m.status = s
	m.statusKind = k
	m.statusSeq++
	seq := m.statusSeq
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg {
		return clearStatusMsg{seq: seq}
	})
}
