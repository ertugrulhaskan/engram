package tui

// The @provider seam: engram hands a project's memory situation to an
// interactive AI session rather than building an in-app repair UI, because the
// assistant already knows how to read files, ask questions and edit — and a
// repair UI would have to reinvent all of it badly (SPEC §8.1).
//
// Every provider here is driven the same way: launch its CLI *interactively*
// with a seed prompt describing what engram was showing, suspend the TUI while
// it runs, and reload from disk when it exits. A CLI that can only take a
// prompt non-interactively does not belong in this table — the handoff is the
// whole feature, and a one-shot answer would be a different product.
//
// The second half of that rule is easy to overlook: a provider must also be
// able to reach the memory directory. engram normally launches in the project
// dir, and the memories live under ~/.claude, outside it — so an entry whose
// invocation drops addDir would open a session pointed at files it is not
// allowed to touch. Every entry below therefore carries an add-dir equivalent,
// and TestAssistantArgsCarryAddDir fails if a future one forgets.

import (
	"os/exec"

	tea "github.com/charmbracelet/bubbletea"
)

// assistant is one AI CLI reachable through "@" in the palette.
type assistant struct {
	key     string // matched against the text after "@"
	label   string // primary palette line
	sub     string // secondary muted line
	bin     string // executable to find on $PATH
	rules   string // the instruction file this assistant reads, named in the seed prompt
	install string // where to get it, for the not-found message
	// args builds an INTERACTIVE invocation carrying the seed prompt. addDir is
	// a directory outside cwd the assistant must also be allowed to touch, or ""
	// when cwd already covers it. Every provider has some form of the flag; the
	// closure exists because the spelling and the safe argument *order* differ.
	args func(prompt, addDir string) []string
}

// assistants is the registry. Each entry's invocation is verified against that
// tool's own documentation or source — never from memory — because a guessed
// flag produces a launch that fails, or worse, silently runs non-interactively.
// Verified 2026-08-30:
//
//   - claude   — `--add-dir`, prompt positional after `--`.
//   - gemini   — docs/reference/configuration.md: `--prompt-interactive` (-i)
//     "starts an interactive session with the provided prompt as the initial
//     input"; `-p` is the non-interactive one. `--include-directories` is its
//     add-dir, granting read+write via the sandbox's allowedPaths.
//   - codex    — codex-rs/tui/src/cli.rs: the TUI takes an optional positional
//     PROMPT ("Optional user prompt to start the session"); `codex exec` is the
//     non-interactive sibling. Its TuiSharedCliOptions is a newtype over
//     SharedCliOptions whose augment_args delegates straight through, so the
//     interactive TUI accepts `--add-dir` ("Additional directories that should
//     be writable alongside the primary workspace"), not just `codex exec`.
//   - copilot  — `-i, --interactive <prompt>` "Start interactive mode and
//     automatically execute this prompt", beside a `--add-dir <directory>`;
//     `-p/--prompt` is its non-interactive form. (Checked against the binary,
//     GitHub Copilot CLI 1.0.82.)
var assistants = []assistant{
	{
		key: "claude", label: "@Claude", sub: "fix & edit memories/plans with Claude Code",
		bin: "claude", rules: "CLAUDE.md", install: "https://claude.com/claude-code",
		// "--" always ends option parsing, so the multi-line seed prompt is taken
		// as the positional [prompt] in every launch mode — never swallowed by the
		// variadic --add-dir (which would fail at startup with ENAMETOOLONG) and
		// never misread as a flag if the prompt text ever changes.
		args: func(prompt, addDir string) []string {
			var a []string
			if addDir != "" {
				a = append(a, "--add-dir", addDir)
			}
			return append(a, "--", prompt)
		},
	},
	{
		key: "gemini", label: "@Gemini", sub: "fix & edit memories/plans with the Gemini CLI",
		bin: "gemini", rules: "GEMINI.md", install: "https://github.com/google-gemini/gemini-cli",
		// --include-directories accepts a repeated *or* comma-separated list, so
		// it is the one flag here that may parse greedily. It goes last, after
		// -i has already consumed the prompt as its own value, so there is no
		// trailing token for it to absorb.
		args: func(prompt, addDir string) []string {
			a := []string{"-i", prompt}
			if addDir != "" {
				a = append(a, "--include-directories", addDir)
			}
			return a
		},
	},
	{
		key: "codex", label: "@Codex", sub: "fix & edit memories/plans with the Codex CLI",
		bin: "codex", rules: "AGENTS.md", install: "https://github.com/openai/codex",
		// Same shape as claude: the prompt is positional, so "--" ends option
		// parsing ahead of it rather than trusting --add-dir's arity.
		args: func(prompt, addDir string) []string {
			var a []string
			if addDir != "" {
				a = append(a, "--add-dir", addDir)
			}
			return append(a, "--", prompt)
		},
	},
	{
		key: "copilot", label: "@Copilot", sub: "fix & edit memories/plans with GitHub Copilot CLI",
		bin: "copilot", rules: ".github/copilot-instructions.md", install: "https://github.com/github/copilot-cli",
		// --add-dir takes exactly one directory per occurrence, so the prompt
		// stays safe as -i's value regardless of order.
		args: func(prompt, addDir string) []string {
			a := []string{"-i", prompt}
			if addDir != "" {
				a = append(a, "--add-dir", addDir)
			}
			return a
		},
	},
}

// lookPath resolves an assistant binary. A package var so tests can simulate
// "installed" / "not installed" without mutating $PATH.
var lookPath = func(bin string) string { return firstInPath(bin) }

// findAssistant returns the registry entry for a key.
func findAssistant(key string) (assistant, bool) {
	for _, a := range assistants {
		if a.key == key {
			return a, true
		}
	}
	return assistant{}, false
}

// installedAssistants are the ones whose CLI is on $PATH. The palette lists
// only these: offering an assistant the user doesn't have would be advertising
// an action that can't run, the same rule the status bar's offered action
// follows. When none is installed the palette still lists them all, so "@" is
// never an empty section that explains nothing — selecting one then gives the
// install hint.
//
// Called once, at New, and kept on the Model (installedAsst): each call sweeps
// $PATH four times, and the palette rebuilds on every keystroke. The cost of
// resolving once is that a CLI installed mid-session appears at the next launch
// rather than immediately — a fair trade for keeping the event loop off the
// filesystem, and the same answer the whole session then agrees on.
func installedAssistants() []assistant {
	var out []assistant
	for _, a := range assistants {
		if lookPath(a.bin) != "" {
			out = append(out, a)
		}
	}
	if len(out) == 0 {
		return assistants
	}
	return out
}

// assistantCmd launches the chosen provider: an interactive session seeded with
// the selected project's memory/plan health, reloading when it exits. It reuses
// the same suspend/resume handoff editCmd uses for $EDITOR.
func (m *Model) assistantCmd(provider string) tea.Cmd {
	a, ok := findAssistant(provider)
	if !ok {
		return m.setDanger("unknown assistant: " + provider)
	}
	bin := lookPath(a.bin)
	if bin == "" {
		return m.setDanger(a.bin + " CLI not found on PATH — install it: " + a.install)
	}
	cwd, memDir, projDir, unresolved := m.assistantContext()
	prompt := m.buildSeedPrompt(a, projDir, memDir, unresolved)
	// Only grant the memory dir explicitly when it isn't already under cwd (the
	// project-dir launch); in the ~/.claude/projects fallback it's already inside.
	addDir := memDir
	if within(memDir, cwd) {
		addDir = ""
	}
	c := exec.Command(bin, a.args(prompt, addDir)...)
	c.Dir = cwd
	// Swap-seam: a future "new window / embedded pane" run mode replaces only
	// this line — command construction, cwd, and the seed prompt are reusable.
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return assistantFinishedMsg{label: a.label, err: err}
	})
}
