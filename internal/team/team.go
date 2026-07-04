// Package team manages engram's shared team store: a git repository, cloned into
// the user's config directory, that holds memories shared across a team. It
// shells out to git and touches the filesystem; it contains no UI code.
package team

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

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

// containsSymlink reports whether any existing path component of full that lies
// below root is a symbolic link. promote and pull call it before writing to or
// reading a store path, to refuse acting *through* a symlink a teammate may have
// committed into the shared repo — one pointing at, say, ~/.ssh or a shell rc
// would turn a promote into an arbitrary-file overwrite or a pull into an
// arbitrary read. Fresh clones also disable core.symlinks so such links never
// materialize; this guard is filesystem/git-version independent and additionally
// protects clones made before that hardening. Components are checked shallowest
// first, so the outermost symlink is caught before it is traversed.
func containsSymlink(root, full string) bool {
	rel, err := filepath.Rel(root, full)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false // full is not under root — not a path this guard owns
	}
	cur := root
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		if part == "" {
			continue
		}
		cur = filepath.Join(cur, part)
		fi, err := os.Lstat(cur)
		if err != nil {
			return false // this component doesn't exist yet — nothing to traverse
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			return true
		}
	}
	return false
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
