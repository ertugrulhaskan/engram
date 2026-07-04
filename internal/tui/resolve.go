package tui

import (
	"os/exec"

	tea "github.com/charmbracelet/bubbletea"
)

// resolveFinishedMsg reports that the $EDITOR conflict-resolution session exited.
type resolveFinishedMsg struct {
	memPath string
	tmpPath string
	err     error
}

// resolveCmd opens the conflict temp file in $EDITOR (suspending the TUI, the same
// handoff editCmd uses) and reports back so the edited result can be applied.
func (m Model) resolveCmd(memPath, tmpPath string) tea.Cmd {
	parts := m.resolveEditor()
	args := append([]string{}, parts[1:]...)
	args = append(args, tmpPath)
	c := exec.Command(parts[0], args...)
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return resolveFinishedMsg{memPath: memPath, tmpPath: tmpPath, err: err}
	})
}
