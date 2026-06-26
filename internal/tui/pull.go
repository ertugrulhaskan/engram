package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ertugrulhaskan/engram/internal/team"
)

// pullFinishedMsg reports the outcome of a background Pull.
type pullFinishedMsg struct {
	res team.PullResult
	err error
}

// pullCmd resolves each local project's team key and pulls project-scoped team
// memories into the matching dirs, off the UI thread. The (dir, memDir) pairs are
// snapshotted on the UI thread; the git remote lookups (ProjectKey) and the pull
// itself run in the background.
func (m Model) pullCmd() tea.Cmd {
	type proj struct{ dir, memDir string }
	seen := map[string]proj{}
	for _, mm := range m.memories {
		if mm.Project.Dir != "" && mm.Project.MemoryDir != "" {
			seen[mm.Project.Dir] = proj{mm.Project.Dir, mm.Project.MemoryDir}
		}
	}
	projs := make([]proj, 0, len(seen))
	for _, p := range seen {
		projs = append(projs, p)
	}

	return func() tea.Msg {
		var targets []team.ProjectTarget
		for _, p := range projs {
			key, err := team.ProjectKey(p.dir)
			if err != nil || key == "" {
				continue
			}
			targets = append(targets, team.ProjectTarget{Key: key, MemoryDir: p.memDir})
		}
		if len(targets) == 0 {
			return pullFinishedMsg{err: fmt.Errorf("no projects with a git remote to pull into")}
		}
		res, err := team.Pull(targets)
		return pullFinishedMsg{res: res, err: err}
	}
}
