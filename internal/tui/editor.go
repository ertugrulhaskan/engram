package tui

import (
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ertugrulhaskan/engram/internal/config"
)

// --- editing ---

type editorFinishedMsg struct {
	path string
	err  error
}

func (m Model) editCmd(path string) tea.Cmd {
	parts := m.resolveEditor()
	args := append([]string{}, parts[1:]...)
	args = append(args, path)
	c := exec.Command(parts[0], args...)
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return editorFinishedMsg{path: path, err: err}
	})
}

// resolveEditor picks the command (and any args) used to open a file for editing.
// It honors the config editor override first, then $VISUAL and $EDITOR (the Unix
// convention), then the host editor when engram runs inside one (e.g. VS Code's
// integrated terminal — using --wait so the edit completes before reload), then
// a terminal editor, falling back to vi.
func (m Model) resolveEditor() []string {
	if v := strings.TrimSpace(m.editorOverride); v != "" {
		return strings.Fields(v)
	}
	if v := strings.TrimSpace(os.Getenv("VISUAL")); v != "" {
		return strings.Fields(v)
	}
	if v := strings.TrimSpace(os.Getenv("EDITOR")); v != "" {
		return strings.Fields(v)
	}
	if os.Getenv("TERM_PROGRAM") == "vscode" {
		if c := firstInPath("code", "code-insiders", "cursor", "codium"); c != "" {
			return []string{c, "--wait"}
		}
	}
	if c := firstInPath("nvim", "vim", "nano", "vi"); c != "" {
		return []string{c}
	}
	return []string{"vi"}
}

// firstInPath returns the first of names found on $PATH, or "".
func firstInPath(names ...string) string {
	for _, n := range names {
		if p, err := exec.LookPath(n); err == nil {
			return p
		}
	}
	return ""
}

// openSettingsFile ensures the config file exists, then opens it in the editor
// so the user can edit theme/editor as JSON. Settings reload when the editor
// closes (see editorFinishedMsg).
func (m *Model) openSettingsFile() tea.Cmd {
	m.mode = modeNormal
	p, err := config.Path()
	if err != nil {
		return m.setDanger("settings: " + err.Error())
	}
	if _, statErr := os.Stat(p); os.IsNotExist(statErr) {
		_ = config.Save(config.Config{Theme: m.theme().Name, Editor: m.editorOverride})
	}
	return m.editCmd(p)
}
