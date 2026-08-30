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
	if cmd := m.gitMissing(); cmd != nil {
		return m, cmd // without git, "no remote" can't be told from "no git"
	}
	it, ok := m.selected()
	if !ok || it.ProjectDir == "" || it.MemDir == "" {
		return m, m.setStatus("select a memory in a project first")
	}
	if team.IsHomeDir(it.ProjectDir) {
		// Claude's home-folder project is where sessions run outside any repo;
		// keying it would push personal notes into a team project bucket.
		return m, m.setStatus("this is your home folder's memory project, not a repository — it promotes globally")
	}
	name = strings.TrimSpace(name)
	if name == "-" {
		return m.clearAlias(it.MemDir)
	}
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
			return m, m.setStatus("usage: >alias <name> — this project has no git remote, so it promotes globally until it has a key")
		case team.RemoteGone:
			if alias != "" {
				return m, m.setStatus("keyed by " + team.AliasKey(alias) + " — its directory is gone, the alias still applies; clear it with >alias -")
			}
			return m, m.setStatus("this project's directory is gone and it has no alias — it promotes globally")
		}
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
	}
	var clean string
	err = config.Update(func(c *config.Config) error {
		if c.ProjectAliases == nil {
			c.ProjectAliases = map[string]string{}
		}
		if err := team.SetAlias(c.ProjectAliases, it.MemDir, name); err != nil {
			return err
		}
		clean = c.ProjectAliases[it.MemDir]
		return nil
	})
	if err != nil {
		return m, m.setDanger("alias not saved — " + configErr(err))
	}
	m.aliases = cleanAliases(config.Load().ProjectAliases)
	return m, m.setStatus("keyed by " + team.AliasKey(clean) + " — promote and pull use it while this project has no git remote")
}

// errNoAlias is clearAlias's "nothing to clear", kept apart from real failures.
var errNoAlias = errors.New("no alias set for this project")

// clearAlias removes the alias configured for memDir (`>alias -`).
func (m Model) clearAlias(memDir string) (tea.Model, tea.Cmd) {
	err := config.Update(func(c *config.Config) error {
		if _, had := c.ProjectAliases[memDir]; !had {
			return errNoAlias
		}
		delete(c.ProjectAliases, memDir)
		return nil
	})
	switch {
	case errors.Is(err, errNoAlias):
		return m, m.setStatus(err.Error())
	case err != nil:
		return m, m.setDanger("alias not cleared — " + configErr(err))
	}
	m.aliases = cleanAliases(config.Load().ProjectAliases)
	return m, m.setStatus("alias cleared — this project promotes globally until it has a key")
}
