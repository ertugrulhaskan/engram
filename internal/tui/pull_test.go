package tui

import "testing"

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
