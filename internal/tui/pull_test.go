package tui

import (
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ertugrulhaskan/engram/internal/memory"
	"github.com/ertugrulhaskan/engram/internal/team"
)

// A project with no git remote is still a global-pull target. It can receive no
// project-scoped memory — applyPull's byKey map still requires a non-empty key —
// but a global one lands in exactly such a project, because promote falls back to
// global where there is no remote. Dropping it here is what made `p` a dead key
// on a [behind] global row: the status bar offered pull, and pull never visited
// the directory, so the run reported "nothing to pull · N up to date".
func TestResolveTargetsKeepsRemotelessProjects(t *testing.T) {
	projs := []pullProj{{dir: t.TempDir(), memDir: "/mem/remoteless"}}

	targets := resolveTargets(projs)
	if len(targets) != 1 {
		t.Fatalf("targets = %d, want 1 — a remoteless project is still a global-pull target", len(targets))
	}
	if targets[0].Key != "" {
		t.Errorf("Key = %q, want empty for a project with no git remote", targets[0].Key)
	}
	if targets[0].MemoryDir != "/mem/remoteless" {
		t.Errorf("MemoryDir = %q, want the project's memory dir", targets[0].MemoryDir)
	}
}

// The confirmed accounting must be the applied accounting. The plan resolves
// each project's key once and hands the targets to the dialog; y applies those
// targets rather than re-snapshotting, because the 2s poll adopts a changed
// alias map while the dialog is open — a >alias from a second window could
// otherwise key a project the dialog listed as skipped.
func TestApplyPullUsesTheConfirmedTargets(t *testing.T) {
	want := []team.ProjectTarget{{Key: "alias/acme", MemoryDir: "/mem/acme"}}
	tm, _ := Model{}.applyPullPlan(pullPlanMsg{res: team.PullResult{Placed: 1}, targets: want})
	got := tm.(Model)
	if got.mode != modePullConfirm {
		t.Fatalf("mode = %v, want modePullConfirm for a plan with work", got.mode)
	}
	if !reflect.DeepEqual(got.pullTargets, want) {
		t.Fatalf("pullTargets = %+v, want the plan's targets %+v", got.pullTargets, want)
	}
	// esc forgets them: a later y can only follow a fresh plan.
	tm, _ = got.updatePullConfirm(tea.KeyMsg{Type: tea.KeyEsc})
	if tm.(Model).pullTargets != nil {
		t.Errorf("pullTargets survived esc; a cancelled plan must not be applied later")
	}
}

// With no confirmed plan, apply refuses instead of resolving a fresh snapshot —
// the proof that y walks the plan's targets and nothing else. The model holds a
// remoteless project on purpose: a re-snapshot would turn it into a target and
// hand it to the store, which answers "not initialized" here, not this refusal.
func TestApplyPullRefusesWithoutAConfirmedPlan(t *testing.T) {
	gitIsolated(t)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir()) // no real store is ever reached
	m := Model{memories: []memory.Memory{{Project: memory.Project{Dir: t.TempDir(), MemoryDir: "/mem/remoteless"}}}}
	msg := m.applyPullCmd()()
	fin, ok := msg.(pullFinishedMsg)
	if !ok {
		t.Fatalf("msg = %T, want pullFinishedMsg", msg)
	}
	if fin.err == nil || !strings.Contains(fin.err.Error(), "no confirmed pull plan") {
		t.Errorf("err = %v, want the no-confirmed-plan refusal", fin.err)
	}
}
