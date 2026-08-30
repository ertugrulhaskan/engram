package tui

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ertugrulhaskan/engram/internal/config"
	"github.com/ertugrulhaskan/engram/internal/team"
)

// gitIsolated skips without git and keeps the developer's own git config out
// of the test — a url.<base>.insteadOf rewrite would otherwise change what
// `git remote get-url` reports (the identity_test precedent).
func gitIsolated(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
}

// gitRepo makes a repository at a temp dir with the given origin URL ("" for
// none) and returns its path.
func gitRepo(t *testing.T, origin string) string {
	t.Helper()
	repo := t.TempDir()
	args := [][]string{{"init", "-q"}}
	if origin != "" {
		args = append(args, []string{"remote", "add", "origin", origin})
	}
	for _, a := range args {
		cmd := exec.Command("git", a...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", a, err, out)
		}
	}
	return repo
}

// The remote wins; the alias stands in only when git reports no remote; a
// malformed alias (hand-edited config) yields no key rather than an unsafe one;
// and when git cannot say — the directory is missing, or the origin has no
// host/path to key by — there is no key at all, never a stale alias.
func TestProjectKeyAliasFallback(t *testing.T) {
	gitIsolated(t)
	const memDir = "/x/memory"
	m := ready(t)
	m.aliases = map[string]string{memDir: "Acme-App"}

	plain := t.TempDir() // a plain directory: no repository, so no remote
	if got := m.projectKey(plain, memDir); got != "alias/acme-app" {
		t.Errorf("remoteless + alias = %q, want alias/acme-app", got)
	}
	if got := m.projectKey(gitRepo(t, ""), memDir); got != "alias/acme-app" {
		t.Errorf("repo without origin + alias = %q, want alias/acme-app", got)
	}
	if got := m.projectKey(gitRepo(t, "git@github.com:acme/app.git"), memDir); got != "github.com/acme/app" {
		t.Errorf("repo with origin = %q, want the remote key over the alias", got)
	}
	if got := m.projectKey(filepath.Join(plain, "gone"), memDir); got != "alias/acme-app" {
		t.Errorf("vanished directory with an alias = %q, want alias/acme-app (the alias outlives the folder)", got)
	}
	if got := m.projectKey(filepath.Join(plain, "gone"), "/no/alias/here"); got != "" {
		t.Errorf("vanished directory, no alias = %q, want \"\"", got)
	}
	if got := m.projectKey(gitRepo(t, "/srv/repo.git"), memDir); got != "" {
		t.Errorf("origin with no host/path = %q, want \"\" (unkeyable remote is not \"no remote\")", got)
	}
	m.aliases[memDir] = "bad/alias"
	if got := m.projectKey(plain, memDir); got != "" {
		t.Errorf("malformed alias = %q, want \"\" (never an unsafe key)", got)
	}
	if got := m.projectKey(t.TempDir(), "/other/memory"); got != "" {
		t.Errorf("remoteless, no alias = %q, want \"\"", got)
	}
}

// resolveTargets is the pull side of the same rule, from a snapshot: one
// target per memory dir, the remote key when there is one (an alias beside it
// is not a second key — applyPull's tombstone pass visits each target), the
// alias for a remoteless project, and an empty key that keeps the project
// rather than dropping it.
func TestResolveTargetsUsesAlias(t *testing.T) {
	gitIsolated(t)
	targets := resolveTargets([]pullProj{
		{dir: t.TempDir(), memDir: "/x/memory", alias: "acme-app"},
		{dir: t.TempDir(), memDir: "/y/memory"},
		{dir: gitRepo(t, "git@github.com:acme/app.git"), memDir: "/w/memory", alias: "acme-app-old"},
	})
	want := []team.ProjectTarget{
		{Key: "alias/acme-app", MemoryDir: "/x/memory"},
		{Key: "", MemoryDir: "/y/memory"},
		{Key: "github.com/acme/app", MemoryDir: "/w/memory"},
	}
	if !reflect.DeepEqual(targets, want) {
		t.Errorf("resolveTargets = %+v, want %+v", targets, want)
	}
}

// Every team verb starts with the git check; its message has to land on the
// model the verb returns, not on a discarded copy.
func TestGitMissingIsReported(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // nothing on it, so git isn't found
	m := ready(t)
	tm, _ := m.actionPromote()
	if got := tm.(Model).status; !strings.Contains(got, "git not found") {
		t.Errorf("promote without git: status = %q, want the git-not-found line", got)
	}
}

// remoteless points the selected row at a plain directory (no git, so no
// remote) with a memory dir that exists on disk — an alias holder whose memory
// dir is gone is reclaimed, so the dir must be real — and returns that memory
// dir, the alias key.
func remoteless(t *testing.T, m *Model) string {
	t.Helper()
	if _, ok := m.selected(); !ok {
		t.Fatal("setup: nothing selected")
	}
	m.rows[m.cursor].item.ProjectDir = t.TempDir()
	m.rows[m.cursor].item.MemDir = t.TempDir()
	return m.rows[m.cursor].item.MemDir
}

// >alias on a remoteless project: bare reports usage, a bad name is refused
// with the rule, a good one is normalized, persisted under the memory dir, and
// used by promote.
func TestAliasCommand(t *testing.T) {
	gitIsolated(t)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := ready(t)
	memDir := remoteless(t, &m)
	dir := m.rows[m.cursor].item.ProjectDir

	tm, _ := m.actionAlias("")
	if got := tm.(Model).status; !strings.Contains(got, "usage: >alias <name>") {
		t.Errorf("bare >alias status = %q, want the usage line", got)
	}
	tm, _ = m.actionAlias("bad/alias")
	if got := tm.(Model).status; !strings.Contains(got, "letters, digits") {
		t.Errorf("bad alias status = %q, want the rule", got)
	}

	tm, _ = m.actionAlias("  Acme-App ")
	got := tm.(Model)
	if got.aliases[memDir] != "acme-app" {
		t.Errorf("aliases[%q] = %q, want acme-app (normalized, keyed by memory dir)", memDir, got.aliases[memDir])
	}
	if saved := config.Load().ProjectAliases[memDir]; saved != "acme-app" {
		t.Errorf("persisted alias = %q, want acme-app", saved)
	}
	if !strings.Contains(got.status, "keyed by alias/acme-app") {
		t.Errorf("set status = %q, want the new key", got.status)
	}
	if key := got.projectKey(dir, memDir); key != "alias/acme-app" {
		t.Errorf("projectKey after >alias = %q, want alias/acme-app", key)
	}
	tm, _ = got.actionAlias("")
	if st := tm.(Model).status; !strings.Contains(st, "keyed by alias/acme-app") {
		t.Errorf("bare >alias after setting = %q, want the current key", st)
	}

	// `>alias -` clears it, on disk and in the session.
	tm, _ = got.actionAlias("-")
	cleared := tm.(Model)
	if _, still := cleared.aliases[memDir]; still || config.Load().ProjectAliases[memDir] != "" {
		t.Errorf("after >alias -: aliases=%v persisted=%q, want none", cleared.aliases, config.Load().ProjectAliases[memDir])
	}
	if !strings.Contains(cleared.status, "alias cleared") {
		t.Errorf("clear status = %q", cleared.status)
	}
	tm, _ = cleared.actionAlias("-")
	if st := tm.(Model).status; st != "no alias set for this project" {
		t.Errorf("clearing twice: status = %q", st)
	}
}

// Claude's home-folder project is not a repository, but keying it would push
// personal notes into a team project bucket; it is refused by name.
func TestAliasRefusedForHomeProject(t *testing.T) {
	gitIsolated(t)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip(err)
	}
	m := ready(t)
	if _, ok := m.selected(); !ok {
		t.Fatal("setup: nothing selected")
	}
	m.rows[m.cursor].item.ProjectDir = home
	tm, _ := m.actionAlias("personal")
	got := tm.(Model)
	if !strings.Contains(got.status, "home folder") || len(got.aliases) != 0 {
		t.Errorf("status = %q aliases = %v, want the home-folder refusal and nothing stored", got.status, got.aliases)
	}
}

// A project that has a git remote is refused by name: the alias would never be
// consulted there, and saying so beats a silent no-op. A project git cannot
// answer for — an origin with no host/path — is refused too, with the cause.
func TestAliasRefusedWhenRemoteExists(t *testing.T) {
	gitIsolated(t)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := ready(t)
	if _, ok := m.selected(); !ok {
		t.Fatal("setup: nothing selected")
	}
	m.rows[m.cursor].item.ProjectDir = gitRepo(t, "git@github.com:acme/app.git")
	tm, _ := m.actionAlias("acme-app")
	got := tm.(Model)
	if !strings.Contains(got.status, "keyed by github.com/acme/app") {
		t.Errorf("status = %q, want the refusal naming the remote key", got.status)
	}
	if len(got.aliases) != 0 || len(config.Load().ProjectAliases) != 0 {
		t.Errorf("an alias was stored for a project with a remote: %v", got.aliases)
	}

	m.rows[m.cursor].item.ProjectDir = gitRepo(t, "/srv/repo.git")
	tm, _ = m.actionAlias("acme-app")
	got = tm.(Model)
	if !strings.Contains(got.status, "can't tell whether this project has a remote") {
		t.Errorf("unkeyable origin: status = %q, want the can't-tell refusal", got.status)
	}
	if len(got.aliases) != 0 {
		t.Errorf("an alias was stored for a project git couldn't answer for: %v", got.aliases)
	}

	// A directory that is gone can't be given a new alias either — the
	// refusal says an existing one still applies — and bare >alias there
	// reports the alias still in effect rather than the refusal.
	m.rows[m.cursor].item.ProjectDir = filepath.Join(t.TempDir(), "gone")
	tm, _ = m.actionAlias("acme-app")
	got = tm.(Model)
	if !strings.Contains(got.status, "directory is gone") || len(got.aliases) != 0 {
		t.Errorf("vanished dir: status = %q aliases = %v, want the gone refusal and nothing stored", got.status, got.aliases)
	}
	m.aliases = map[string]string{m.rows[m.cursor].item.MemDir: "acme-app"}
	tm, _ = m.actionAlias("")
	if st := tm.(Model).status; !strings.Contains(st, "keyed by alias/acme-app") || !strings.Contains(st, "still applies") {
		t.Errorf("bare >alias on a vanished dir: status = %q, want the alias still in effect", st)
	}
}

// One alias, one project: a second project asking for a name already in use
// is refused, since pull would otherwise place both projects' memories into
// each other.
func TestAliasRefusedWhenTaken(t *testing.T) {
	gitIsolated(t)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := ready(t)
	memDir := remoteless(t, &m)
	tm, _ := m.actionAlias("acme-app")
	got := tm.(Model)
	if got.aliases[memDir] != "acme-app" {
		t.Fatalf("setup: first alias not stored: %v", got.aliases)
	}
	got.rows[got.cursor].item.ProjectDir = t.TempDir()
	elsewhere := t.TempDir()
	got.rows[got.cursor].item.MemDir = elsewhere
	tm, _ = got.actionAlias("ACME-app") // same alias after normalization
	after := tm.(Model)
	if !strings.Contains(after.status, "already keys "+memDir) {
		t.Errorf("status = %q, want the refusal naming the project that holds the alias", after.status)
	}
	if _, stored := after.aliases[elsewhere]; stored {
		t.Errorf("a duplicate alias was stored: %v", after.aliases)
	}

	// A holder whose memory dir is gone still holds the store bucket its
	// memories were shared under, so the name is not reclaimed silently — the
	// refusal says how to free it.
	stale := filepath.Join(t.TempDir(), "gone", "memory")
	cfg := config.Load()
	cfg.ProjectAliases = map[string]string{stale: "acme-app"}
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	got.aliases = cleanAliases(cfg.ProjectAliases)
	tm, _ = got.actionAlias("acme-app")
	kept := tm.(Model)
	if !strings.Contains(kept.status, "already keys "+stale) || !strings.Contains(kept.status, "projectAliases") {
		t.Errorf("gone holder: status = %q, want the refusal naming it and the way out", kept.status)
	}
	if _, stored := kept.aliases[elsewhere]; stored {
		t.Errorf("the name was reassigned over a gone holder: %v", kept.aliases)
	}
}

// A config that does not parse is refused, not replaced: writing defaults over
// it would wipe the user's other settings and call it success.
func TestAliasRefusesUnparseableConfig(t *testing.T) {
	gitIsolated(t)
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	p := filepath.Join(home, "engram", "config.json")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	broken := `{"theme": "crt", "scanRoots": ["~/work"],}` // trailing comma
	if err := os.WriteFile(p, []byte(broken), 0o644); err != nil {
		t.Fatal(err)
	}
	m := ready(t)
	remoteless(t, &m)
	tm, _ := m.actionAlias("acme-app")
	got := tm.(Model)
	if !strings.Contains(got.status, "doesn't parse") {
		t.Errorf("status = %q, want the parse refusal", got.status)
	}
	if b, _ := os.ReadFile(p); string(b) != broken {
		t.Errorf("config.json was rewritten:\n%s", b)
	}
	if len(got.aliases) != 0 {
		t.Errorf("alias stored despite the refusal: %v", got.aliases)
	}
}

// `>alias <name>` typed in the palette carries the name through to the action.
func TestPaletteAliasArg(t *testing.T) {
	gitIsolated(t)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	mm := ready(t)
	remoteless(t, &mm)
	var m tea.Model = mm
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	m = typeRunes(m, ">alias Nimbus")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := m.(Model)
	if got.mode != modeNormal {
		t.Errorf("mode after enter = %v, want modeNormal", got.mode)
	}
	if !strings.Contains(got.status, "keyed by alias/nimbus") {
		t.Errorf("status = %q, want keyed by alias/nimbus", got.status)
	}
}
