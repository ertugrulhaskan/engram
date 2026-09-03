package tui

import (
	"os/exec"
	"strings"
	"testing"
)

// Every registry entry must pass the memory dir through to its CLI. engram
// normally launches in the project dir, with the memories under ~/.claude
// outside it, so an entry that drops addDir opens a session pointed at files it
// cannot touch — a silent failure the user only discovers mid-repair. This is
// the test that caught gemini and codex ignoring it.
func TestAssistantArgsCarryAddDir(t *testing.T) {
	const prompt, addDir = "SEED", "/mem"
	for _, a := range assistants {
		t.Run(a.key, func(t *testing.T) {
			args := a.args(prompt, addDir)

			at := indexOf(args, addDir)
			if at < 0 {
				t.Fatalf("%s drops addDir: %v", a.key, args)
			}
			// It has to arrive as a flag's value, not as a bare positional the
			// CLI would read as part of the prompt.
			if at == 0 || !strings.HasPrefix(args[at-1], "-") {
				t.Errorf("%s passes addDir without a flag: %v", a.key, args)
			}
			if indexOf(args, prompt) < 0 {
				t.Errorf("%s drops the seed prompt: %v", a.key, args)
			}

			// With no addDir the flag must vanish entirely, not linger with an
			// empty value that the CLI would reject at startup.
			bare := a.args(prompt, "")
			if indexOf(bare, "") >= 0 {
				t.Errorf("%s emits an empty argument with no addDir: %v", a.key, bare)
			}
			if indexOf(bare, prompt) < 0 {
				t.Errorf("%s drops the seed prompt with no addDir: %v", a.key, bare)
			}
		})
	}
}

// nonInteractiveVerbs are the subcommands a CLI uses for its one-shot mode.
// codex's non-interactive form is `codex exec`, a positional token — not a
// flag — so a flag-only check waves it straight through. A subcommand can only
// come first, so that is the only position worth refusing.
var nonInteractiveVerbs = map[string]bool{"exec": true, "run": true}

// nonInteractive reports the token that would drop a built invocation to a
// one-shot answer, or "" when there is none. seed and memDir are the two values
// an invocation legitimately carries, so every *other* argument is inspected —
// deciding "is this a flag position?" by exclusion rather than by looking at
// the argument before it, which treated the prompt after a "--" separator as
// that separator's value and skipped it, exactly where claude and codex put it.
func nonInteractive(args []string, seed, memDir string) string {
	for i, arg := range args {
		if arg == seed || arg == memDir {
			continue // a value, not a flag: "-p" here would be prompt text
		}
		switch arg {
		case "-p", "--prompt", "--print":
			return arg
		}
		if i == 0 && nonInteractiveVerbs[arg] {
			return arg
		}
	}
	return ""
}

// The seam exists to hand off to an *interactive* session; every CLI here also
// has a non-interactive form that would silently turn the handoff into a
// one-shot answer. None of them may appear in a built invocation.
func TestAssistantArgsAreInteractive(t *testing.T) {
	const (
		seed   = "SEED"
		memDir = "/mem"
	)
	for _, a := range assistants {
		args := a.args(seed, memDir)
		if bad := nonInteractive(args, seed, memDir); bad != "" {
			t.Errorf("%s would run non-interactively (%q): %v", a.key, bad, args)
		}
	}
}

// The check above only means anything if it actually inspects every position a
// non-interactive token can occupy. These are the shapes it must catch — each
// one a form the guard it replaced let through.
func TestAssistantArgsCheckEveryPosition(t *testing.T) {
	const (
		seed   = "SEED"
		memDir = "/mem"
	)
	for _, shape := range [][]string{
		{"--", "-p", seed},                           // after a separator: the position the old form skipped
		{"-p", seed},                                 // leading
		{"--add-dir", memDir, "-p", seed},            // after a flag's value
		{"-i", seed, "--print", "--add-dir", memDir}, // after the prompt
		{"exec", "--add-dir", memDir, "--", seed},    // codex's one-shot subcommand — no flag check sees it
	} {
		if nonInteractive(shape, seed, memDir) == "" {
			t.Errorf("a planted non-interactive form went unnoticed in %v", shape)
		}
	}
	// And it must not fire on a legitimate invocation. "exec" anywhere but
	// position 0 is a value, not a subcommand.
	for _, fine := range [][]string{
		{"--add-dir", memDir, "--", seed},
		{"-i", seed, "--include-directories", memDir},
		{"--add-dir", "exec", "--", seed},
	} {
		if bad := nonInteractive(fine, seed, memDir); bad != "" {
			t.Errorf("false positive %q on a legitimate invocation %v", bad, fine)
		}
	}
}

// The palette and the not-found message read every one of these fields, so a
// half-filled entry would render a blank row or an install hint pointing nowhere.
func TestAssistantRegistryComplete(t *testing.T) {
	seen := map[string]bool{}
	for _, a := range assistants {
		if seen[a.key] {
			t.Errorf("duplicate assistant key %q — findAssistant would shadow one", a.key)
		}
		seen[a.key] = true

		if a.key == "" || a.bin == "" || a.sub == "" || a.rules == "" || a.args == nil {
			t.Errorf("assistant %+v has an empty field", a)
		}
		if !strings.HasPrefix(a.label, "@") {
			t.Errorf("%s label %q must start with @ — buildSeedPrompt strips it", a.key, a.label)
		}
		if !strings.HasPrefix(a.install, "https://") {
			t.Errorf("%s install hint %q is not a URL", a.key, a.install)
		}
	}
}

func indexOf(args []string, want string) int {
	for i, a := range args {
		if a == want {
			return i
		}
	}
	return -1
}

// The claude entry's exact argv, pinned: --add-dir before the "--" that keeps
// the multi-line prompt positional. The generic guards above prove every
// provider carries addDir; this one proves the shipped spelling of one of them.
func TestBuildClaudeCmd(t *testing.T) {
	c := claudeCmdFor(t, "SEED", "/mem")
	if c.Dir != "/proj" {
		t.Errorf("cwd = %q, want /proj", c.Dir)
	}
	want := []string{"claude", "--add-dir", "/mem", "--", "SEED"}
	if strings.Join(c.Args, "\x00") != strings.Join(want, "\x00") {
		t.Errorf("args = %v, want %v", c.Args, want)
	}

	// No add-dir → still terminate options with "--" so the prompt stays positional.
	c2 := claudeCmdFor(t, "SEED", "")
	want2 := []string{"claude", "--", "SEED"}
	if strings.Join(c2.Args, "\x00") != strings.Join(want2, "\x00") {
		t.Errorf("args (no addDir) = %v, want %v", c2.Args, want2)
	}
}

// claudeAssistant is the registry's claude entry, for tests that need one.
func claudeAssistant(t *testing.T) assistant {
	t.Helper()
	a, ok := findAssistant("claude")
	if !ok {
		t.Fatal("the claude assistant is missing from the registry")
	}
	return a
}

// claudeCmdFor builds the claude invocation the registry would run, so the
// argv assertions below test the shipped construction rather than a copy.
func claudeCmdFor(t *testing.T, prompt, addDir string) *exec.Cmd {
	t.Helper()
	a := claudeAssistant(t)
	c := exec.Command("claude", a.args(prompt, addDir)...)
	c.Dir = "/proj"
	return c
}

// With nothing installed the list falls back to every provider — but as a copy.
// Handing back the package-level slice would let anything that later appends to
// or sorts the Model's copy rewrite the registry every other reader shares.
func TestInstalledAssistantsFallbackIsACopy(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // no CLI resolves
	got := installedAssistants()
	if len(got) != len(assistants) {
		t.Fatalf("fallback returned %d providers, want all %d", len(got), len(assistants))
	}
	first := assistants[0]
	got[0] = assistant{key: "mutated"}
	if assistants[0].key != first.key {
		t.Errorf("writing the returned slice rewrote the registry: assistants[0] is now %q", assistants[0].key)
	}
}
