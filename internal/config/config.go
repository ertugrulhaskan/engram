// Package config persists engram's user settings (theme, editor) as JSON under
// the XDG config directory. It contains no UI code.
//
// An *absent* file means defaults; an *unparseable* one does not. Read tells the
// two apart (ErrUnparseable), because the file now carries placement decisions —
// projectAliases — and answering a file engram cannot read with defaults would
// promote an aliased project's memories into global/ off a file it never saw.
// Update is the one write path and refuses over a file that did not parse, so no
// writer can replace the user's settings with defaults; Save is atomic, so no
// writer can create the unparseable file in the first place. Write failures stay
// non-fatal to the caller, which reports them.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
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
	// team.AliasKey turns it into the store key (alias/<name>). team.ResolveKey
	// is the one rule promote and pull share: the alias applies while git reports
	// no origin remote (team.ErrNoRemote) and while the project directory has
	// gone — never on any other failure, so a hiccup can't let a stale alias
	// stand in for a real remote, and never on the home folder (RemoteHome). A
	// project that later gains a remote switches to its remote key on its own;
	// memories already promoted under the alias stay in that bucket until
	// promoted again.
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
// "present but unparseable" (ErrUnparseable). It is the only reader: there is
// deliberately no error-dropping convenience wrapper, because every caller has
// to decide what an unreadable file means for what it is about to do. A writer
// goes through Update.
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

// Save writes the config, creating the directory if needed. The write is
// atomic — a temp file beside it, then a rename — because os.WriteFile
// truncates before it writes, and engram writes this file often: every 1/2/3
// theme keypress and every >alias. Interrupted between the truncate and the
// write, it used to leave a half-object behind, which Read reports as
// ErrUnparseable and main.go treats as fatal; the repair it advertises
// (/settings) lives inside the TUI that would no longer start. A rename within
// one directory is atomic, so a reader sees the whole old file or the whole new
// one, never a prefix.
//
// It does not fsync. The failure this exists to stop is a *process* death
// mid-write — a crash, a SIGKILL, an ENOSPC — and against that the rename is
// complete on its own: the bytes are in the page cache and every later reader
// sees them. fsync would buy only the power-loss case, and buy it by blocking
// the event loop on every theme keypress (config.Update is called inline from
// the key handler), which on a network home is the worse trade for a settings
// file whose worst-case loss is one theme preference. Both ext4 and APFS order
// the data before a rename-over anyway.
//
// The trade-off worth stating: the rename needs write permission on the
// *directory*, which os.WriteFile did not once the file existed. A read-only
// config dir holding a writable config.json now fails to save rather than
// saving non-atomically — the rarer situation, and the better failure.
func Save(c Config) error {
	p, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	// Follow a symlinked config before choosing where the temp lives: the temp
	// has to sit in the same directory as the file the rename replaces, since a
	// rename cannot cross filesystems.
	p = resolveConfigPath(p)
	dir := filepath.Dir(p)
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	// Decide the mode before creating anything, so the temp is never wider than
	// the file it will become — not even for the moment between the write and a
	// corrective chmod.
	mode, existed := targetMode(p)
	f, tmp, err := createTemp(dir, mode)
	if err != nil {
		return err
	}
	defer os.Remove(tmp) // a no-op once the rename below has consumed it
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	// The open above was narrowed by the umask, which is right for a file being
	// created and wrong for one being replaced: os.WriteFile applied its perm
	// only at creation, so a mode the user set survived every later write. Put
	// it back exactly.
	if existed {
		if err := os.Chmod(tmp, mode); err != nil {
			return err
		}
	}
	return os.Rename(tmp, p)
}

// targetMode is the mode Save's output should end up with, and whether the
// config already exists. An existing file's own permissions are carried across
// the rename; a new one gets 0644 for the umask to narrow, exactly as it
// narrowed the file os.WriteFile used to create. Perm() masks to the 0777 bits,
// so setuid/setgid/sticky are never copied.
func targetMode(p string) (os.FileMode, bool) {
	st, err := os.Stat(p)
	if err != nil {
		return 0o644, false
	}
	return st.Mode().Perm(), true
}

// resolveConfigPath follows a symlinked config to the file it points at, so a
// config.json that a dotfiles manager (stow, chezmoi) links into a repo keeps
// receiving engram's writes. os.WriteFile wrote *through* the link; a rename
// would replace the link with a regular file and silently strand the repo copy.
//
// A *dangling* link is followed too, which EvalSymlinks alone will not do: it
// fails outright when the target is missing, and falling back to the link path
// would then have the rename eat the link. That is not a contrived state — it
// is the normal bootstrap order, the link made before the repo copy exists —
// and os.WriteFile handled it by opening through the link and creating the
// target. Readlink gives the same answer, with a relative target resolved
// against the link's own directory, as the kernel resolves it.
//
// Only one hop is followed, and only when the link itself resolves to nothing:
// a link chain that works is EvalSymlinks' job.
func resolveConfigPath(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	st, err := os.Lstat(p)
	if err != nil || st.Mode()&os.ModeSymlink == 0 {
		return p // not a link at all: absent, or a real file EvalSymlinks tripped on
	}
	target, err := os.Readlink(p)
	if err != nil {
		return p
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(p), target)
	}
	// The target's directory has to exist for the rename to land in it. If it
	// does not, there is nothing better to do than write the link's own path.
	if st, err := os.Stat(filepath.Dir(target)); err != nil || !st.IsDir() {
		return p
	}
	return target
}

// createTemp makes a uniquely-named file beside the config, open for writing,
// and returns it with its path. It is not os.CreateTemp: that hardcodes 0600,
// and there is no portable way to read the umask back and widen it correctly
// afterwards. Opening at the mode the file will end up with lets the umask
// narrow a *new* config exactly as it narrowed the file os.WriteFile used to
// create — so a user on umask 077 keeps getting a private config, rather than a
// world-readable one holding their editor command, scan roots and project
// aliases — and never leaves the temp wider than its destination.
//
// O_EXCL is what makes the predictable name safe: a pre-planted file or symlink
// at that name fails the open with EEXIST and the loop moves on, so the write
// can never be redirected through one.
func createTemp(dir string, mode os.FileMode) (*os.File, string, error) {
	for i := 0; i < 1000; i++ {
		name := filepath.Join(dir, fmt.Sprintf(".config-%d-%d.tmp", os.Getpid(), i))
		f, err := os.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_EXCL, mode)
		if err == nil {
			return f, name, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return nil, "", err
		}
	}
	return nil, "", fmt.Errorf("no free temp filename in %s", dir)
}
