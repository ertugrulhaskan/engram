// Package team manages engram's shared team store: a git repository, cloned into
// the user's config directory, that holds memories shared across a team. It
// shells out to git and touches the filesystem; it contains no UI code.
package team

import (
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
