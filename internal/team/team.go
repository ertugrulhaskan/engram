// Package team manages engram's shared team store: a git repository, cloned into
// the user's config directory, that holds memories shared across a team. It
// shells out to git and touches the filesystem; it contains no UI code.
package team

import (
	"os"
	"os/exec"
	"path/filepath"

	"github.com/ertugrulhaskan/engram/internal/config"
)

// HasGit reports whether the git executable is available on PATH. Every team
// operation shells out to git, so callers gate on it to show a clear, actionable
// message instead of a raw "executable file not found" exec error.
func HasGit() bool {
	_, err := exec.LookPath("git")
	return err == nil
}

// Dir returns the managed team-store location, <config dir>/team — i.e.
// $XDG_CONFIG_HOME/engram/team, falling back to ~/.config/engram/team.
func Dir() (string, error) {
	base, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "team"), nil
}

// IsInitialized reports whether the team store has been set up — i.e. a clone
// exists at Dir() (an `engram init-team` has run). Callers use this to gate
// sharing actions and surface a clear "run init-team first" message.
func IsInitialized() bool {
	dir, err := Dir()
	if err != nil {
		return false
	}
	_, err = os.Stat(filepath.Join(dir, ".git"))
	return err == nil
}
