package tui

// >alias keys a project that has no git remote by a name the user assigns, so
// its memories can be promoted to and pulled into a project bucket instead of
// collecting in global/. The name lives in the config (projectAliases, memory
// dir → alias) and is read through team.CleanAliases; the store key it stands
// for is team.AliasKey, and the fallback rule — remote first, alias only when
// git reports there is none or the directory is gone — is team.ResolveKey,
// shared by promote and pull. A project that later gains a remote promotes
// under its remote key from then on; what was shared under the alias stays in
// that bucket (pull reports it skipped) until promoted again.

import (
	"errors"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ertugrulhaskan/engram/internal/config"
	"github.com/ertugrulhaskan/engram/internal/team"
)

// projectKey resolves a project's store key through team.ResolveKey with the
// alias configured for its memory dir.
func (m Model) projectKey(dir, memDir string) string {
	key, _ := team.ResolveKey(dir, m.aliases[memDir])
	return key
}

// cleanAliases is what the model holds: the config's projectAliases through
// team.CleanAliases, dropped entries discarded (the /settings reload is where
// they are reported).
func cleanAliases(raw map[string]string) map[string]string {
	clean, _ := team.CleanAliases(raw)
	return clean
}

// configErr words a config write that was refused, adding the way out when
// the file itself is the problem.
func configErr(err error) string {
	if errors.Is(err, config.ErrUnparseable) {
		return err.Error() + " — fix it via /settings"
	}
	return err.Error()
}

// actionAlias handles `>alias [name]` for the selected memory's project. With
// a name it assigns the alias and persists it; `-` clears it; bare, it reports
// the key in effect and why. A project that has a git remote is refused by
// naming its real key — an alias there would never be consulted, and a silent
// no-op is worse than a refusal.
func (m Model) actionAlias(name string) (tea.Model, tea.Cmd) {
	if !m.caps().Share {
		return m.denyShare("alias")
	}
	it, ok := m.selected()
	if !ok || it.ProjectDir == "" || it.MemDir == "" {
		return m, m.setStatus("select a memory in a project first")
	}
	name = strings.TrimSpace(name)
	// Clearing comes before every refusal below, git included. An alias
	// hand-edited into the config for a project those refusals cover — the home
	// folder, one whose remote engram can't read, or any project at all on a
	// machine without git — could otherwise never be removed from inside engram,
	// only by editing config.json. A guard on *setting* a value is not a reason
	// to trap the value already there, and removing a config entry needs neither
	// git nor a remote. (The source gate above stays first: it says where you
	// are, and switching source is a keystroke.)
	if name == "-" {
		return m.clearAlias(it.MemDir)
	}
	if cmd := m.gitMissing(); cmd != nil {
		return m, cmd // without git, "no remote" can't be told from "no git"
	}
	// The home folder is refused below as team.RemoteHome, not by a check of
	// its own: an alias is a name invented for a project with no identity of
	// its own, and Claude's home-folder project — sessions run outside any
	// repository — isn't one. A real remote still keys it, and arrives here as
	// RemoteFound like any other, so only the alias is refused.
	key, state, err := team.ClassifyRemote(it.ProjectDir)
	alias := m.aliases[it.MemDir]
	if name == "" { // report the key in effect
		switch state {
		case team.RemoteFound:
			return m, m.setStatus("keyed by " + key + " (git remote) — an alias would never be used")
		case team.RemoteNone:
			if alias != "" {
				return m, m.setStatus("keyed by " + team.AliasKey(alias) + " — change it with >alias <name>, clear it with >alias -")
			}
			// The config may well name an alias for this project that
			// CleanAliases threw out — malformed, or claimed by two memory
			// dirs. Saying "no key" there is actively false, and leaves the
			// user no way inside engram to learn why what they wrote isn't
			// applying, since the /settings warning is a one-shot they may
			// have missed.
			if len(m.aliasDropped) > 0 {
				return m, m.setDanger("no key in effect — projectAliases entries were ignored: " + strings.Join(m.aliasDropped, "; "))
			}
			return m, m.setStatus("usage: >alias <name> — this project has no git remote, so it promotes globally until it has a key")
		case team.RemoteGone:
			if alias != "" {
				return m, m.setStatus("keyed by " + team.AliasKey(alias) + " — its directory is gone, the alias still applies; clear it with >alias -")
			}
			return m, m.setStatus("this project's directory is gone and it has no alias — it promotes globally")
		case team.RemoteHome:
			return m, m.setStatus("your home folder's memory project can't take an alias — those memories promote globally")
		case team.RemoteReserved:
			return m, m.setDanger("this project's remote host is engram's reserved \"alias\" namespace — rename the ssh alias to key it")
		}
		// RemoteUnknown, and any state added without a case above: this switch
		// and the one below must stay in step, so both end in the same fallback
		// rather than one of them silently returning nothing.
		return m, m.setDanger("can't tell whether this project has a remote — " + err.Error())
	}
	switch state {
	case team.RemoteFound:
		return m, m.setStatus("this project has a git remote — keyed by " + key + ", so an alias would never be used")
	case team.RemoteGone:
		// An alias already set keeps working (team.ResolveKey); a new one can't
		// be checked against a remote that may have existed there.
		return m, m.setDanger("this project's directory is gone (" + it.ProjectDir + ") — an alias already set still applies; a new one can't be set")
	case team.RemoteUnknown:
		// An origin whose URL can't be keyed, a repository git won't read: an
		// alias would be a guess about a project we can't see.
		return m, m.setDanger("can't tell whether this project has a remote — " + err.Error())
	case team.RemoteReserved:
		// git answered; engram refused the answer, because this origin would key
		// straight into the alias namespace. An alias here would land in the very
		// bucket the refusal exists to keep unambiguous, so it is refused too —
		// and the message names the real fix rather than blaming git.
		return m, m.setDanger("this project's remote host is engram's reserved \"alias\" namespace — rename the ssh alias rather than aliasing it here")
	case team.RemoteHome:
		return m, m.setStatus("your home folder's memory project can't take an alias — those memories promote globally")
	case team.RemoteNone:
		// The one state an alias is for; everything below writes it.
	default:
		// A state added without a case here must not reach the write: an alias
		// stored against a project engram can't classify is exactly the stale
		// key team.ResolveKey refuses to honour.
		return m, m.setDanger("can't tell whether this project has a remote — no alias set")
	}
	var clean string
	var saved map[string]string
	err = config.Update(func(c *config.Config) error {
		if c.ProjectAliases == nil {
			c.ProjectAliases = map[string]string{}
		}
		if err := team.SetAlias(c.ProjectAliases, it.MemDir, name); err != nil {
			return err
		}
		clean = c.ProjectAliases[it.MemDir]
		// Take the map Update just wrote rather than reading the file back: a
		// re-read that failed and was answered with a zero Config would
		// silently empty every alias this session holds — promote and pull
		// would fall back to global — while the status line below reported the
		// new key. This map is what is on disk.
		saved = c.ProjectAliases
		return nil
	})
	if err != nil {
		return m, m.setDanger("alias not saved — " + configErr(err))
	}
	m.aliases = cleanAliases(saved)
	// A poll tick already in flight read the file before this write landed;
	// bumping the generation makes the handler discard it rather than let the
	// pre-write snapshot undo what the status line is about to report.
	m.settingsGen++
	return m, m.setStatus("keyed by " + team.AliasKey(clean) + " — promote and pull use it while this project has no git remote")
}

// errNoAlias is clearAlias's "nothing to clear", kept apart from real failures.
var errNoAlias = errors.New("no alias set for this project")

// clearAlias removes the alias configured for memDir (`>alias -`).
func (m Model) clearAlias(memDir string) (tea.Model, tea.Cmd) {
	var saved map[string]string
	err := config.Update(func(c *config.Config) error {
		if _, had := c.ProjectAliases[memDir]; !had {
			return errNoAlias
		}
		delete(c.ProjectAliases, memDir)
		saved = c.ProjectAliases // the map Update wrote — see actionAlias
		return nil
	})
	switch {
	case errors.Is(err, errNoAlias):
		return m, m.setStatus(err.Error())
	case err != nil:
		return m, m.setDanger("alias not cleared — " + configErr(err))
	}
	m.aliases = cleanAliases(saved)
	// A poll tick already in flight read the file before this write landed;
	// bumping the generation makes the handler discard it rather than let the
	// pre-write snapshot undo what the status line is about to report.
	m.settingsGen++
	return m, m.setStatus("alias cleared — this project promotes globally until it has a key")
}
