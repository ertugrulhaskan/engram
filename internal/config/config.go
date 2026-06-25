// Package config persists engram's user settings (theme, editor) as JSON under
// the XDG config directory. It contains no UI code. Settings are best-effort: a
// missing or unreadable file just means defaults, and write failures are
// non-fatal.
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Config is engram's persisted settings.
type Config struct {
	Theme  string `json:"theme,omitempty"`  // theme name, e.g. "Nord"
	Editor string `json:"editor,omitempty"` // optional editor command override, e.g. "code --wait"
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

// Load reads the config, returning a zero Config when it is absent or unreadable.
func Load() Config {
	var c Config
	p, err := Path()
	if err != nil {
		return c
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return c
	}
	_ = json.Unmarshal(data, &c) // corrupt file → defaults
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
