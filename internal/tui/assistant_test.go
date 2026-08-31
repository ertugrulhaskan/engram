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

// The seam exists to hand off to an *interactive* session; every CLI here also
// has a non-interactive prompt flag that would silently turn the handoff into a
// one-shot answer. None of them may appear in a built invocation.
func TestAssistantArgsAreInteractive(t *testing.T) {
	for _, a := range assistants {
		args := a.args("SEED", "/mem")
		for i, arg := range args {
			// Only flag positions count — "-p" as a *value* would be prompt text.
			if i > 0 && !strings.HasPrefix(args[i-1], "-") || i == 0 {
				if arg == "-p" || arg == "--prompt" {
					t.Errorf("%s uses the non-interactive prompt flag %q: %v", a.key, arg, args)
				}
			}
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
