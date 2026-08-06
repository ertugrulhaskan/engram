package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ertugrulhaskan/engram/internal/team"
)

// pullFinishedMsg reports the outcome of a background pull apply.
type pullFinishedMsg struct {
	res team.PullResult
	err error
}

// pullProj is one local project snapshot: its dir (for the git-remote lookup)
// and its memory dir (the pull target).
type pullProj struct{ dir, memDir string }

// snapshotProjects captures each local project's (dir, memoryDir) pair on the
// UI thread; the git remote lookups run in the background afterwards.
func (m Model) snapshotProjects() []pullProj {
	seen := map[string]pullProj{}
	for _, mm := range m.memories {
		if mm.Project.Dir != "" && mm.Project.MemoryDir != "" {
			seen[mm.Project.Dir] = pullProj{mm.Project.Dir, mm.Project.MemoryDir}
		}
	}
	projs := make([]pullProj, 0, len(seen))
	for _, p := range seen {
		projs = append(projs, p)
	}
	return projs
}

// resolveTargets turns snapshotted projects into store keys (background work —
// each lookup shells out to git).
func resolveTargets(projs []pullProj) []team.ProjectTarget {
	var targets []team.ProjectTarget
	for _, p := range projs {
		key, err := team.ProjectKey(p.dir)
		if err != nil || key == "" {
			continue
		}
		targets = append(targets, team.ProjectTarget{Key: key, MemoryDir: p.memDir})
	}
	return targets
}

// planPullCmd fetches the store and computes the pull accounting off the UI
// thread — nothing local moves until the plan is confirmed.
func (m Model) planPullCmd() tea.Cmd {
	projs := m.snapshotProjects()
	return func() tea.Msg {
		targets := resolveTargets(projs)
		if len(targets) == 0 {
			return pullPlanMsg{err: fmt.Errorf("no projects with a git remote to pull into")}
		}
		res, err := team.PullPlan(targets)
		return pullPlanMsg{res: res, err: err}
	}
}

// applyPullCmd applies a confirmed plan (no second fetch, so the confirmed
// accounting is the applied accounting), off the UI thread.
func (m Model) applyPullCmd() tea.Cmd {
	projs := m.snapshotProjects()
	return func() tea.Msg {
		targets := resolveTargets(projs)
		if len(targets) == 0 {
			return pullFinishedMsg{err: fmt.Errorf("no projects with a git remote to pull into")}
		}
		res, err := team.PullApply(targets)
		return pullFinishedMsg{res: res, err: err}
	}
}
