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

// pullProj is one local project snapshot: its dir (for the git-remote lookup),
// its memory dir (the pull target), and its configured alias, if any.
type pullProj struct{ dir, memDir, alias string }

// snapshotProjects captures each local project's (dir, memoryDir) pair on the
// UI thread; the git remote lookups run in the background afterwards.
func (m Model) snapshotProjects() []pullProj {
	// Keyed by memory dir, the project's stable identity: two memory dirs can
	// decode to one best-effort project dir, and each is its own pull target.
	seen := map[string]pullProj{}
	for _, mm := range m.memories {
		if mm.Project.Dir != "" && mm.Project.MemoryDir != "" {
			seen[mm.Project.MemoryDir] = pullProj{mm.Project.Dir, mm.Project.MemoryDir, m.aliases[mm.Project.MemoryDir]}
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
//
// A project with no git remote is keyed by its alias when one is configured
// (>alias — team.ResolveKey, the same rule promote uses; the alias comes from
// the map team.CleanAliases already validated), and otherwise keeps an empty
// Key rather than being dropped. With no key it can receive no *project*-scoped
// memory — there is nothing to match one against, and applyPull's byKey map
// still requires a non-empty key — but it can hold a global one, and a
// remoteless project is exactly where global memories collect, since promote
// falls back to global there. Dropping it here is what made `p` a dead key on
// a [behind] global row: the status bar offered pull, and pull never visited
// the directory. One target per memory dir: applyPull's tombstone pass visits
// each target, so a second key for the same dir would double its accounting.
func resolveTargets(projs []pullProj) []team.ProjectTarget {
	var targets []team.ProjectTarget
	for _, p := range projs {
		key, _ := team.ResolveKey(p.dir, p.alias)
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
			return pullPlanMsg{err: fmt.Errorf("no local projects to pull into")}
		}
		res, err := team.PullPlan(targets)
		return pullPlanMsg{res: res, targets: targets, err: err}
	}
}

// applyPullCmd applies a confirmed plan off the UI thread: no second fetch and
// no second key resolution, so the confirmed accounting is the applied
// accounting. Re-resolving here would read the alias map as it is *now*, and
// the 2s poll adopts config changes while the dialog is open — a >alias from a
// second window could then key a project the plan listed as skipped.
func (m Model) applyPullCmd() tea.Cmd {
	targets := m.pullTargets
	return func() tea.Msg {
		if len(targets) == 0 {
			return pullFinishedMsg{err: fmt.Errorf("no confirmed pull plan to apply")}
		}
		res, err := team.PullApply(targets)
		return pullFinishedMsg{res: res, err: err}
	}
}
