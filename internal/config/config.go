// Package config persists engram's user settings (theme, editor) as JSON under
// the XDG config directory. It contains no UI code. Settings are best-effort: a
// missing or unreadable file just means defaults, and write failures are
// non-fatal.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Config is engram's persisted settings.
type Config struct {
	Theme  string `json:"theme,omitempty"`  // theme key: "midnight" (default) | "paperback" | "crt"
	Editor string `json:"editor,omitempty"` // optional editor command override, e.g. "code --wait"

	// Secret-scan guard on promote. Empty means the default.
	SecretScanAction string `json:"secretScanAction,omitempty"` // "block" (default) | "block-strict" | "warn" | "off"
	SecretScanScope  string `json:"secretScanScope,omitempty"`  // "secrets" (default) | "secrets+pii"

	// ScanRoots are extra directories to look for projects in, beyond the ones
	// Claude Code already knows about under ~/.claude/projects. Each root and its
	// immediate children are checked; a directory counts as a project only when it
	// carries an instruction file (CLAUDE.md, AGENTS.md, GEMINI.md, or
	// .github/copilot-instructions.md). A leading "~" is expanded, and a root
	// must be absolute once expanded — a relative one would resolve against
	// wherever engram happened to be launched from, so it is ignored.
	//
	// Depth is deliberately 1: this is re-checked on every poll tick, so a
	// recursive walk would run several times a minute for the life of the session.
	ScanRoots []string `json:"scanRoots,omitempty"`

	// ProjectAliases key the projects that have no git remote to derive a team
	// key from: memory dir → alias. Keyed by the memory dir rather than the
	// project dir because the memory dir is the project's stable identity — the
	// project dir is decoded best-effort from Claude's folder name and can
	// change when the tree does. team.NormalizeAlias validates an alias and
	// team.AliasKey turns it into the store key (alias/<name>). Promote and pull
	// consult the alias only while git reports no origin remote (team.ErrNoRemote,
	// not any failure), so a project that later gains a remote switches to its
	// remote key on its own — memories already promoted under the alias stay in
	// that bucket until promoted again.
	ProjectAliases map[string]string `json:"projectAliases,omitempty"`
}

// ScanAction returns the configured promote-time secret-scan action, defaulting
// to "block" (block with an informed override) for empty or unrecognized values.
func (c Config) ScanAction() string {
	switch c.SecretScanAction {
	case "block-strict", "warn", "off":
		return c.SecretScanAction
	default:
		return "block"
	}
}

// ScanPII reports whether the scanner should also flag PII (emails, card-like
// numbers). Off by default — PII false-positives constantly in real memories.
func (c Config) ScanPII() bool {
	return c.SecretScanScope == "secrets+pii"
}

// Dir returns engram's config directory: $XDG_CONFIG_HOME/engram, falling back
// to ~/.config/engram. Other packages (e.g. internal/team) build their managed
// paths under this directory.
func Dir() (string, error) {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "engram"), nil
}

// Path returns the config file location: $XDG_CONFIG_HOME/engram/config.json,
// falling back to ~/.config/engram/config.json.
func Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

// ErrUnparseable is what Read returns (wrapped, naming the file and the JSON
// error) for a config that exists but does not parse — so a caller can tell
// it from every other failure and say what to do about it.
var ErrUnparseable = errors.New("the config doesn't parse")

// Read parses the config, telling "absent" (zero Config, nil error) apart from
// "present but unparseable" (ErrUnparseable). Load is the read-only
// convenience; a writer goes through Update.
func Read() (Config, error) {
	var c Config
	p, err := Path()
	if err != nil {
		return c, err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return c, nil
		}
		return c, err
	}
	if err := json.Unmarshal(data, &c); err != nil {
		return c, fmt.Errorf("%w (%s: %v)", ErrUnparseable, p, err)
	}
	return c, nil
}

// Update applies mutate to the config on disk and saves the result. It reads
// through Read, so a file that does not parse is refused rather than replaced
// with defaults, and every setting the mutation doesn't touch survives the
// round trip — the invariant lives here, in the one function that can break
// it, not at each writer. mutate may refuse too; its error comes back unsaved.
func Update(mutate func(*Config) error) error {
	c, err := Read()
	if err != nil {
		return err
	}
	if err := mutate(&c); err != nil {
		return err
	}
	return Save(c)
}

// Load reads the config for reading only, treating absent and unreadable alike
// as defaults.
func Load() Config {
	c, _ := Read()
	return c
}

// Save writes the config, creating the directory if needed.
func Save(c Config) error {
	p, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o644)
}
