// Package team manages engram's shared team store: a git repository, cloned into
// the user's config directory, that holds memories shared across a team. It
// shells out to git and touches the filesystem; it contains no UI code.
package team

import (
	"os"
	"path/filepath"

	"github.com/ertugrulhaskan/engram/internal/config"
)

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
