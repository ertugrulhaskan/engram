package tui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ertugrulhaskan/engram/internal/memory"
)

// --- @Claude assistant ---

// assistantFinishedMsg is delivered when the launched assistant session exits;
// it carries the process error (nil on a clean exit). The handler reloads from
// disk so any memories/plans the assistant touched show immediately.
type assistantFinishedMsg struct{ err error }

// lookClaude resolves the claude binary on $PATH. It's a package var so tests
// can simulate "not installed" without mutating $PATH.
var lookClaude = func() string { return firstInPath("claude") }

// seedListCap bounds how many drifted filenames get spelled out in the seed
// prompt, so a pathological directory doesn't produce a giant CLI argument.
const seedListCap = 10

// assistantCmd dispatches to the chosen provider. Only "claude" exists today;
// the provider seam keeps the door open for other assistants (v3) without
// touching the palette.
func (m *Model) assistantCmd(provider string) tea.Cmd {
	switch provider {
	case "claude":
		return m.claudeCmd()
	default:
		return m.setDanger("unknown assistant: " + provider)
	}
}

// claudeCmd launches an interactive Claude Code session, seeded with the
// selected project's memory/plan health, then reloads when it exits. It reuses
// the same suspend/resume handoff editCmd uses for $EDITOR.
func (m *Model) claudeCmd() tea.Cmd {
	bin := lookClaude()
	if bin == "" {
		return m.setDanger("claude CLI not found on PATH — install Claude Code: https://claude.com/claude-code")
	}
	cwd, memDir, projDir, unresolved := m.assistantContext()
	prompt := m.buildSeedPrompt(projDir, memDir, unresolved)
	// Only grant the memory dir explicitly when it isn't already under cwd (the
	// project-dir launch); in the ~/.claude/projects fallback it's already inside.
	addDir := memDir
	if within(memDir, cwd) {
		addDir = ""
	}
	c := buildClaudeCmd(bin, cwd, prompt, addDir)
	// Swap-seam: a future "new window / embedded pane" run mode replaces only
	// this line — command construction, cwd, and the seed prompt are reusable.
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return assistantFinishedMsg{err: err}
	})
}

// buildClaudeCmd assembles the interactive invocation: the seed prompt is the
// trailing positional argument (no -p, so the session is interactive), cwd is
// where Claude reads CLAUDE.md / recalls memories, and addDir (when set) grants
// tool access to a directory outside cwd — the memory dir, when launching in the
// project dir rather than under ~/.claude.
func buildClaudeCmd(bin, cwd, prompt, addDir string) *exec.Cmd {
	var args []string
	if addDir != "" {
		args = append(args, "--add-dir", addDir)
	}
	// "--" always ends option parsing, so the multi-line seed prompt is taken as
	// the positional [prompt] in every launch mode — never swallowed by the
	// variadic --add-dir (which would fail at startup with ENAMETOOLONG) and never
	// misread as a flag if the prompt text ever changes. Verified: `claude --
	// "<prompt>"` starts an interactive session even with no preceding options.
	args = append(args, "--", prompt)
	c := exec.Command(bin, args...)
	c.Dir = cwd
	return c
}

// assistantContext decides where to launch the assistant, reading the SELECTED
// item's own dirs (not a memory-list fallback) so launching from /files on the
// global CLAUDE.md doesn't borrow an unrelated project. When the project dir
// resolves and exists, launch there so Claude reads its CLAUDE.md and recalls its
// memories. A selection with no project of its own (the global ~/.claude/CLAUDE.md
// or a plan) launches in ~/.claude. Otherwise — a renamed/moved folder, or a
// project key that can't be reversed to a real path (a "." in the folder name
// flattens to "-" ambiguously) — launch in the ~/.claude/projects root: inside
// .claude, broad enough to fix or relocate memories across project keys without
// extra trust prompts, and far narrower than $HOME. unresolved is true when a
// project dir was expected but couldn't be resolved; it only softens the prompt.
func (m Model) assistantContext() (cwd, memDir, projDir string, unresolved bool) {
	if it, ok := m.selected(); ok {
		memDir = it.MemDir
		projDir = it.ProjectDir
	}
	if memDir == "" && projDir == "" {
		return claudeHome(), "", "", false
	}
	if memDir == "" { // a project context without its own memory dir → use the active one
		memDir = m.currentMemDir()
	}
	if projDir != "" && dirExists(projDir) {
		return projDir, memDir, projDir, false
	}
	unresolved = projDir != ""
	if root := projectsRoot(memDir); root != "" {
		return root, memDir, projDir, unresolved
	}
	return claudeHome(), memDir, projDir, unresolved
}

// claudeHome is the ~/.claude directory (parent of projects/), used as the launch
// dir when the selection has no project. "" if $HOME is unknown.
func claudeHome() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".claude")
}

// projectsRoot returns the ~/.claude/projects directory that contains a memory
// dir (memDir is .../projects/<encoded-project>/memory, so up two levels). Guards
// against a shallow/malformed memDir resolving to "/" or ".", falling back to
// ~/.claude/projects; returns "" only if $HOME is unknown too.
func projectsRoot(memDir string) string {
	if memDir != "" {
		if r := filepath.Dir(filepath.Dir(memDir)); r != "/" && r != "." {
			return r
		}
	}
	if h := claudeHome(); h != "" {
		return filepath.Join(h, "projects")
	}
	return ""
}

// within reports whether path is base itself or nested under it.
func within(path, base string) bool {
	if path == "" || base == "" {
		return false
	}
	if path == base {
		return true
	}
	return strings.HasPrefix(path, strings.TrimRight(base, string(os.PathSeparator))+string(os.PathSeparator))
}

// buildSeedPrompt is the initial message engram hands the assistant so it
// "already knows" the situation: scope, locations, a live index-health snapshot
// (memories only), and — when relevant — the orphan/migration story. It reads
// drift data from internal/memory; assembling the string here is UI work.
func (m Model) buildSeedPrompt(projDir, memDir string, unresolved bool) string {
	var b strings.Builder
	src := "memories"
	switch m.srcKind {
	case srcPlans:
		src = "plans"
	case srcFiles:
		src = "CLAUDE.md / MEMORY.md files"
	}
	b.WriteString("You've been launched from engram — a TUI for browsing the Claude Code memory and plan files under ~/.claude — to help maintain them.\n\n")
	b.WriteString("Scope: work only on Claude Code memory files and plan-mode plans. Ask before editing any file, and don't touch unrelated project source code.\n\n")

	if memDir != "" {
		fmt.Fprintf(&b, "Memory directory: %s\n", memDir)
	}
	if projDir != "" {
		fmt.Fprintf(&b, "Project: %s\n", projDir)
	}
	fmt.Fprintf(&b, "The user was browsing their %s.\n\n", src)

	// Index drift applies to memories only (plans have no index). Stay silent
	// rather than fabricate a health claim if the dir can't be read.
	if memDir != "" {
		un, dang, err := memory.IndexDrift(memDir)
		switch {
		case err != nil:
		case len(un) == 0 && len(dang) == 0:
			b.WriteString("The MEMORY.md index for this project is currently in sync with the files on disk.\n\n")
		default:
			b.WriteString("The MEMORY.md index is out of sync with the files on disk:\n")
			if len(un) > 0 {
				fmt.Fprintf(&b, "- %d file(s) on disk have no index line: %s\n", len(un), joinCapped(un, seedListCap))
			}
			if len(dang) > 0 {
				fmt.Fprintf(&b, "- %d index entr(y/ies) point at a missing file: %s\n", len(dang), joinCapped(dang, seedListCap))
			}
			b.WriteString("\n")
		}
	}

	if unresolved {
		b.WriteString("NOTE: engram couldn't resolve this project's working directory from its stored project key — the folder may have been renamed or moved, or its name may contain characters (like a \".\") that the key flattens to \"-\" and can't be reversed. ")
		if projDir != "" {
			fmt.Fprintf(&b, "Its best guess was %q, which doesn't exist on disk. ", projDir)
		}
		fmt.Fprintf(&b, "The memory files are at %s; they may already be in the right place. Only if they are genuinely misfiled (Claude Code recalls them for the wrong project, or not at all), offer to relocate them — with their MEMORY.md — to the correct ~/.claude/projects/<encoded-path>/memory key.\n\n", memDir)
	}

	b.WriteString("You can repair index drift, fix malformed frontmatter or broken [[links]], and also create, rewrite, merge, or reorganize memories and plans on request.\n")
	return b.String()
}

// joinCapped joins items with ", ", collapsing the tail past limit into a count.
func joinCapped(items []string, limit int) string {
	if len(items) <= limit {
		return strings.Join(items, ", ")
	}
	return strings.Join(items[:limit], ", ") + fmt.Sprintf(", …and %d more", len(items)-limit)
}

// dirExists reports whether p is an existing directory. Used to decide whether
// a selection's project dir still exists (vs. a renamed/orphaned folder).
func dirExists(p string) bool {
	if p == "" {
		return false
	}
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}
