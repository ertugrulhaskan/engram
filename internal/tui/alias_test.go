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
// remote), with a real memory dir on disk because ClassifyRemote stats the
// project dir before it spawns git — a missing one answers RemoteGone, not
// RemoteNone, which is a different branch from the one these tests exercise.
// It returns that memory dir, which is the alias key.
//
// Nothing here reclaims a name: team.SetAlias refuses a name another memory dir
// holds without ever stating that dir, precisely so a holder whose directory is
// gone keeps the store bucket its memories were shared under until the user
// frees the name deliberately (asserted below, and in SPEC §7).
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
	if saved := loadCfg().ProjectAliases[memDir]; saved != "acme-app" {
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
	if _, still := cleared.aliases[memDir]; still || loadCfg().ProjectAliases[memDir] != "" {
		t.Errorf("after >alias -: aliases=%v persisted=%q, want none", cleared.aliases, loadCfg().ProjectAliases[memDir])
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
	if len(got.aliases) != 0 || len(loadCfg().ProjectAliases) != 0 {
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
	cfg := loadCfg()
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

// The home-folder refusal guards *setting* an alias; it must not trap one that
// is already in the config. `>alias -` clears it there like anywhere else --
// otherwise a hand-edited entry for that project could only be removed by
// editing config.json, since every path into it inside engram is refused.
func TestAliasClearWorksOnARefusedProject(t *testing.T) {
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
	memDir := m.rows[m.cursor].item.MemDir
	if memDir == "" {
		t.Fatal("setup: the selected row has no memory dir")
	}
	m.rows[m.cursor].item.ProjectDir = home
	// Seeded the way a hand edit would, bypassing actionAlias's own guards.
	if err := config.Save(config.Config{ProjectAliases: map[string]string{memDir: "personal"}}); err != nil {
		t.Fatal(err)
	}
	m.aliases = cleanAliases(loadCfg().ProjectAliases)

	tm, _ := m.actionAlias("-")
	got := tm.(Model)
	if len(got.aliases) != 0 {
		t.Errorf("aliases = %v, want the entry cleared", got.aliases)
	}
	if !strings.Contains(got.status, "cleared") {
		t.Errorf("status = %q, want the cleared line", got.status)
	}
	if left := loadCfg().ProjectAliases[memDir]; left != "" {
		t.Errorf("config still holds %q for the home project", left)
	}
}

// The poll's scanRoots fallback is primed from the config New already read, so
// a config broken before the first 2s tick keeps the roots the session started
// with. Unseeded it was nil until that tick, and a break inside the window
// dropped every scanRoots-discovered file out of /files -- the degradation the
// fallback exists to prevent, in a narrower window.
func TestScanRootsFallbackIsSeededAtStartup(t *testing.T) {
	lastGood.Lock()
	saved := lastGood.cfg
	lastGood.Unlock()
	defer seedConfig(saved)

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	roots := []string{"/tmp/acme", "/tmp/widgets"}
	_ = New(sampleMemories(), nil, nil, config.Config{ScanRoots: roots})

	// Break the file so currentScanRoots' own read fails and only the seed answers.
	p, err := config.Path()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := currentScanRoots(); !reflect.DeepEqual(got, roots) {
		t.Errorf("currentScanRoots() = %v, want the seeded %v", got, roots)
	}
}

// A remote whose host is engram's own "alias" namespace is refused — but as
// itself, not as a git failure. git answered correctly here; only engram
// declined the answer, so the message has to name the real cause and the fix
// (rename the ssh alias) rather than sending the user to debug git.
func TestAliasReservedHostIsNotBlamedOnGit(t *testing.T) {
	gitIsolated(t)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := ready(t)
	if _, ok := m.selected(); !ok {
		t.Fatal("setup: nothing selected")
	}
	m.rows[m.cursor].item.ProjectDir = gitRepo(t, "alias:acme")

	for _, arg := range []string{"whatever", ""} {
		tm, _ := m.actionAlias(arg)
		got := tm.(Model).status
		if strings.Contains(got, "can't tell whether") {
			t.Errorf("actionAlias(%q): status blames git for an answer git gave: %q", arg, got)
		}
		if !strings.Contains(got, "reserved") {
			t.Errorf("actionAlias(%q): status = %q, want the reserved-namespace refusal", arg, got)
		}
	}
}

// The scope dialog may only offer >alias where >alias will actually run.
// RemoteNone is the one state it runs in; the home-folder project reaches
// ProjectKey identically to any remoteless project, so team.ClassifyRemote
// gives it RemoteHome to keep the two apart -- otherwise the dialog sends the
// user to a command that answers "can't". Same rule the controls row and the
// status bar's offer follow.
//
// The zero value is checked here too, because it is what an unset promoteState
// would be: exactly one state may carry the offer, so a state nobody set can
// never carry it.
func TestNoKeyLineOffersAliasOnlyWhereItRuns(t *testing.T) {
	const lead = "this project has no key to promote under"
	if got := noKeyLine(lead, team.RemoteNone); !strings.Contains(got, ">alias") {
		t.Errorf("a remoteless project should be offered >alias: %q", got)
	}
	if got := noKeyLine(lead, team.RemoteHome); strings.Contains(got, ">alias") {
		t.Errorf("the home-folder project must not be offered >alias, which refuses it: %q", got)
	}
	// The states that were already right, pinned so they stay that way.
	if got := noKeyLine(lead, team.RemoteReserved); strings.Contains(got, ">alias") || !strings.Contains(got, "reserved") {
		t.Errorf("a reserved host names itself and offers no alias: %q", got)
	}
	if got := noKeyLine(lead, team.RemoteUnknown); strings.Contains(got, ">alias") {
		t.Errorf("git couldn't say, so no alias would be consulted: %q", got)
	}
	var unset team.RemoteState
	if got := noKeyLine(lead, unset); strings.Contains(got, ">alias") {
		t.Errorf("an unset state must not carry the offer: %q", got)
	}
	// Every state names a reason; a state added without a case still lands on
	// the one that promises nothing.
	for _, st := range []team.RemoteState{team.RemoteNone, team.RemoteGone, team.RemoteReserved, team.RemoteHome, team.RemoteUnknown, team.RemoteState(99)} {
		if got := noKeyLine(lead, st); !strings.HasPrefix(got, lead+" — ") || !strings.Contains(got, "globally") {
			t.Errorf("noKeyLine(%v) = %q, want the lead and where it promotes", st, got)
		}
	}
}

// projectAliases is the one setting that decides *placement*, and it used to
// refresh only on the way back from /settings. An alias set by a second engram,
// or hand-edited into the config, therefore left this session keying the
// project as if it had none -- >promote and >pull would place its memories in
// global/ while the scope dialog said "this project has no key to promote
// under". The poll already re-read scanRoots every tick; that it did not
// re-read the aliases was the asymmetry.
func TestPollRefreshesAliases(t *testing.T) {
	lastGood.Lock()
	saved := lastGood.cfg
	lastGood.Unlock()
	defer seedConfig(saved)

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := ready(t)
	if len(m.aliases) != 0 {
		t.Fatalf("setup: session already holds aliases %v", m.aliases)
	}
	// A tick that read the config adopts its map, even with the signature
	// unchanged — this is a settings change, not a filesystem one.
	next, _ := m.Update(pollResultMsg{sig: m.fsSig, parsed: true, aliases: map[string]string{"/a/memory": "acme-app"}})
	got := next.(Model)
	if got.aliases["/a/memory"] != "acme-app" {
		t.Errorf("aliases = %v, want the tick's map", got.aliases)
	}
	// A tick that could NOT read the config keeps what the session has, rather
	// than emptying the map and silently promoting to global/.
	next, _ = got.Update(pollResultMsg{sig: got.fsSig, parsed: false, aliases: nil})
	if kept := next.(Model).aliases["/a/memory"]; kept != "acme-app" {
		t.Errorf("an unreadable config emptied the alias map: %v", next.(Model).aliases)
	}
	// And a tick that read a config with the key *absent* clears it. This is the
	// state `>alias -` leaves behind — projectAliases is omitempty, so removing
	// the last alias removes the key — and reading it as "couldn't parse" would
	// pin a deleted alias for the life of every other session.
	next, _ = got.Update(pollResultMsg{sig: got.fsSig, parsed: true, aliases: map[string]string{}})
	if left := next.(Model).aliases["/a/memory"]; left != "" {
		t.Errorf("a cleared alias survived the poll: %v", next.(Model).aliases)
	}
}

// The end-to-end version of the case above, through the real read: `>alias -`
// clears the last alias, the config it writes has no projectAliases key, and
// the next tick must report that as an authoritative empty rather than as a
// failed read.
func TestPollSeesAClearedLastAlias(t *testing.T) {
	lastGood.Lock()
	saved := lastGood.cfg
	lastGood.Unlock()
	defer seedConfig(saved)

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := config.Update(func(c *config.Config) error {
		c.ProjectAliases = map[string]string{"/a/memory": "acme-app"}
		return nil
	}); err != nil {
		t.Fatalf("seed alias: %v", err)
	}
	cfg, parsed := currentConfig()
	if !parsed || cfg.ProjectAliases["/a/memory"] != "acme-app" {
		t.Fatalf("setup: parsed=%v aliases=%v", parsed, cfg.ProjectAliases)
	}
	// Clear it the way clearAlias does: delete the entry and save.
	if err := config.Update(func(c *config.Config) error {
		delete(c.ProjectAliases, "/a/memory")
		return nil
	}); err != nil {
		t.Fatalf("clear alias: %v", err)
	}
	cfg, parsed = currentConfig()
	if !parsed {
		t.Fatal("a config with no projectAliases key must still read as parsed")
	}
	if len(cfg.ProjectAliases) != 0 {
		t.Errorf("ProjectAliases = %v, want none", cfg.ProjectAliases)
	}
}

// A poll tick reads the config first and delivers after a full filesystem scan.
// A >alias handled inside that window would otherwise be undone by the tick's
// pre-write snapshot -- the status line reporting "keyed by alias/acme" while
// the map no longer holds it, and the very next promote placing into global/.
// The generation counter is what discards a tick that straddles a write.
func TestStalePollTickDoesNotUndoAnAliasWrite(t *testing.T) {
	m := ready(t)
	launched := m.settingsGen // the value a tick in flight would carry

	// The session writes an alias while that tick is out.
	m.aliases = map[string]string{"/a/memory": "acme-app"}
	m.settingsGen++

	next, _ := m.Update(pollResultMsg{sig: m.fsSig, parsed: true, gen: launched, aliases: map[string]string{}})
	if got := next.(Model).aliases["/a/memory"]; got != "acme-app" {
		t.Errorf("a stale tick undid the alias write: aliases = %v", next.(Model).aliases)
	}
	// A tick launched after the write is current, and does apply.
	next, _ = m.Update(pollResultMsg{sig: m.fsSig, parsed: true, gen: m.settingsGen, aliases: map[string]string{}})
	if got := next.(Model).aliases["/a/memory"]; got != "" {
		t.Errorf("a current tick was ignored: aliases = %v", next.(Model).aliases)
	}
}

// The scan policy decides whether credentials are caught before a push, and it
// used to be read only at startup and on the way back from /settings -- so a
// second window kept promoting under the policy its session began with. It
// comes off the same poll read as the roots and the aliases now.
func TestPollRefreshesTheScanPolicy(t *testing.T) {
	m := ready(t)
	m.scanAction, m.scanPII = "off", false
	next, _ := m.Update(pollResultMsg{
		sig:    m.fsSig,
		parsed: true,
		gen:    m.settingsGen,
		cfg:    config.Config{SecretScanAction: "block-strict", SecretScanScope: "secrets+pii", Editor: " code --wait "},
	})
	got := next.(Model)
	if got.scanAction != "block-strict" || !got.scanPII {
		t.Errorf("scan policy = %q/%v, want block-strict/true", got.scanAction, got.scanPII)
	}
	if got.editorOverride != "code --wait" {
		t.Errorf("editorOverride = %q, want the trimmed config value", got.editorOverride)
	}
	// The theme is deliberately NOT adopted from a tick: colours changing under
	// the user mid-session is a surprise, and 1-3 and /settings are explicit.
	before := m.themeIdx
	next, _ = m.Update(pollResultMsg{sig: m.fsSig, parsed: true, gen: m.settingsGen, cfg: config.Config{Theme: "crt"}})
	if next.(Model).themeIdx != before {
		t.Error("a poll tick switched the theme")
	}
}

// An alias the config names but CleanAliases threw out must not be reported as
// "this project has no key" -- that is actively false, and the /settings
// warning is a one-shot the user may have missed.
func TestBareAliasNamesADroppedEntry(t *testing.T) {
	m := ready(t)
	_ = remoteless(t, &m)
	m.aliasDropped = []string{"shared is claimed by /c/memory and /d/memory"}
	tm, _ := m.actionAlias("")
	got := tm.(Model).status
	if !strings.Contains(got, "ignored") || !strings.Contains(got, "shared is claimed by") {
		t.Errorf("status = %q, want the dropped-entry explanation", got)
	}
	if strings.Contains(got, "until it has a key") {
		t.Errorf("status still claims the project was never aliased: %q", got)
	}
}
