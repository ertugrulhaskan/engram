package team

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
)

// ErrNoRemote reports that a project has no origin remote to key it by: the
// directory exists and either no repository holds it or its repository has no
// "origin". It is the one ProjectKey failure an alias may stand in for. Any
// other failure means git could not say — a remote whose URL has no host/path
// to normalize, a repository git refuses to read — and falling back there
// would let a stale alias override a real remote whenever git hiccups. A
// missing directory is reported as the stat error (fs.ErrNotExist), which
// ResolveKey treats as its own case.
var ErrNoRemote = errors.New("no git origin remote")

// RemoteState is what kind of answer ClassifyRemote reached.
//
// RemoteUnknown is first so that it, not RemoteFound, is the zero value. A
// RemoteState that was never set is a state nobody asked git about, and the
// dialogs read it to decide what to offer: "git couldn't tell" withholds every
// offer, while RemoteFound asserts a key exists and RemoteNone invites >alias.
// The zero value must be the one that claims least — the same rule source.Caps
// follows, where an undeclared capability grants nothing.
type RemoteState int

const (
	RemoteUnknown  RemoteState = iota // git could not say: an unkeyable origin, a repository git won't read, no git at all
	RemoteFound                       // git reports an origin; the key is its normalized URL
	RemoteNone                        // the directory exists with no repository or no origin — an alias may stand in
	RemoteGone                        // the project directory no longer exists — an alias already granted keeps standing in
	RemoteReserved                    // git answered fine, but the origin's host is engram's own "alias" namespace
	RemoteHome                        // the user's home folder, with no remote of its own — never keyed, by alias or otherwise
)

// ProjectKey resolves a project directory's canonical team key: it reads the
// directory's `origin` remote and normalizes it (see NormalizeRemote). The key is
// how the same project is matched across machines regardless of local clone path.
// It returns an error when the directory has no usable git remote — ResolveKey
// decides whether a user-assigned alias may stand in, and does so only for
// ErrNoRemote and a vanished directory.
func ProjectKey(projectDir string) (string, error) {
	if projectDir == "" {
		return "", fmt.Errorf("no project directory")
	}
	// A stat is microseconds; a git spawn is not. Asking the filesystem first
	// keeps a vanished directory from paying for a subprocess on every pull.
	if st, err := os.Stat(projectDir); err != nil {
		return "", fmt.Errorf("project directory: %w", err)
	} else if !st.IsDir() {
		return "", fmt.Errorf("project directory %s is not a directory", projectDir)
	}
	url, err := runGitCapture(projectDir, "remote", "get-url", "origin")
	if err != nil {
		if noRemote(projectDir, err) {
			return "", fmt.Errorf("%w in %s", ErrNoRemote, projectDir)
		}
		return "", fmt.Errorf("reading the origin remote in %s: %w", projectDir, err)
	}
	return NormalizeRemote(url)
}

// noRemote classifies a failed `git remote get-url origin` by git's exit
// status rather than its message, which is locale-dependent (verified on git
// 2.55; get-url and its exit 2 date from git 2.7): 2 is "no such remote" — a
// repository without origin. 128 is "not a repository", but also "cannot read
// this repository" (dubious ownership, a corrupt .git), and git says 128 for
// both from `rev-parse --git-dir` too. So "no repository holds dir" needs two
// witnesses to agree: git's rev-parse under its own discovery rules (GIT_DIR,
// GIT_CEILING_DIRECTORIES, a worktree's .git file), and no .git entry up the
// tree. Either one seeing a repository means git could not say, which is the
// safe direction — a stale alias must never stand in for a real remote. The
// extra subprocess runs only on the 128 branch.
func noRemote(dir string, err error) bool {
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return false // git didn't run at all
	}
	switch exitErr.ExitCode() {
	case 2:
		return true
	case 128:
		_, rpErr := runGitCapture(dir, "rev-parse", "--git-dir")
		var rpExit *exec.ExitError
		return errors.As(rpErr, &rpExit) && rpExit.ExitCode() == 128 && !insideRepo(dir)
	}
	return false
}

// insideRepo reports whether dir or any ancestor holds a .git entry (a
// directory, or the file a worktree keeps). It only stats <ancestor>/.git on
// the way up; it reads nothing.
//
// The path is made absolute first. A relative one ends its walk at ".", since
// filepath.Dir(".") is ".", so the ancestors above the working directory are
// never looked at — while `git -C` resolves the same relative path against the
// cwd and does keep walking. On a repository git refuses to read, both
// witnesses would then exit 128 with insideRepo wrongly agreeing there is no
// repository, and noRemote would answer true: exactly the inversion its comment
// promises never to allow, letting a stale alias key a project that has a real
// remote. ProjectKey is exported, so the guard has to hold for any caller.
func insideRepo(dir string) bool {
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	for d := filepath.Clean(dir); ; {
		if _, err := os.Stat(filepath.Join(d, ".git")); err == nil {
			return true
		}
		parent := filepath.Dir(d)
		if parent == d {
			return false
		}
		d = parent
	}
}

// ClassifyRemote resolves a project's key and says what kind of answer that
// is, for a caller that needs more than key-or-nothing — the >alias command,
// which refuses differently for each state. err is ProjectKey's error for
// every state but RemoteFound.
//
// The home folder gets a state of its own rather than a flag beside one.
// Claude's home-folder project reaches ProjectKey exactly like any remoteless
// project and comes back ErrNoRemote, yet it is the one project an alias may
// never key — so callers reading RemoteNone alone would offer >alias to a
// project that refuses it. Deciding it here means IsHomeDir is asked once, on
// the goroutine that already pays for a git spawn, instead of once per caller
// on the UI thread.
func ClassifyRemote(dir string) (key string, state RemoteState, err error) {
	key, err = ProjectKey(dir)
	switch {
	case err == nil:
		return key, RemoteFound, nil
	case errors.Is(err, ErrNoRemote), errors.Is(err, fs.ErrNotExist):
		// The two states an alias may stand in for — unless this is the home
		// folder, which none of them may key. A home *with* a remote never
		// arrives here: it returned RemoteFound above, and that key still
		// applies (see ResolveKey).
		if IsHomeDir(dir) {
			return "", RemoteHome, err
		}
		if errors.Is(err, ErrNoRemote) {
			return "", RemoteNone, err
		}
		return "", RemoteGone, err
	case errors.Is(err, ErrReservedHost):
		// Kept apart from RemoteUnknown: git answered, and correctly. Only
		// engram refuses the answer, so the dialogs must not blame git for it —
		// and unlike the other failures this one has a fix the user can act on.
		return "", RemoteReserved, err
	}
	return "", RemoteUnknown, err
}

// ResolveKey is the one fallback rule, for promote and pull alike: the remote
// when git has one; the alias when git reports there is none, or when the
// project directory has gone — the alias was granted while the directory was
// there and had no remote, and its going missing creates none; "" when the
// project has neither, and "" too when git could not say — never a stale alias
// over a real remote. The state comes back with the key so a dialog can say
// which of those it is. The alias is normalized here as well, so a value that
// bypassed CleanAliases still can't become an unsafe key.
//
// An *alias* never keys the user's home folder: an alias is a name invented for
// a project that has no identity of its own, and Claude's home-folder project —
// the memories of sessions run outside any repository — is not a project in that
// sense. ClassifyRemote reports it as RemoteHome, which falls past the alias
// branch below. A real remote does key it, exactly as before: if the home
// directory is itself a dotfiles repo, ClassifyRemote answers RemoteFound,
// promoting from it offers that key and the scope dialog names it. That is
// deliberate and the bucket is not a privacy boundary — a promote copies the
// memory into the shared store whichever bucket it lands in, so refusing the
// remote here would strand already-shared memories to protect nothing.
func ResolveKey(dir, alias string) (string, RemoteState) {
	key, state, _ := ClassifyRemote(dir)
	switch state {
	case RemoteFound:
		return key, state
	case RemoteNone, RemoteGone:
		name, err := NormalizeAlias(alias)
		if err != nil {
			return "", state
		}
		return AliasKey(name), state
	}
	return "", state
}

// IsHomeDir reports whether dir is the user's home folder, comparing cleaned
// paths with symlinks resolved where they resolve — a decoded project dir and
// $HOME can spell one directory two ways.
func IsHomeDir(dir string) bool {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return false
	}
	canon := func(p string) string {
		p = filepath.Clean(p)
		if r, err := filepath.EvalSymlinks(p); err == nil {
			return r
		}
		return p
	}
	return canon(dir) == canon(home)
}
