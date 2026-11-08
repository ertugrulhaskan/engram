package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ertugrulhaskan/engram/internal/config"
	"github.com/ertugrulhaskan/engram/internal/memory"
)

// The ENGR-44 follow-ups to the /files assistant handoff: what the seed prompt
// promises must match what the launched session can actually reach.

// filesModel is a ready model on /files with doc as its only (selected) row.
func filesModel(t *testing.T, doc memory.DocFile) Model {
	t.Helper()
	m0, _ := New(sampleMemories(), nil, []memory.DocFile{doc}, config.Config{}).Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m := switchVia(m0, "/files").(Model)
	if it, ok := m.selected(); !ok || it.Path != doc.Path {
		t.Fatalf("setup: selected %+v (ok=%v), want %s", it, ok, doc.Path)
	}
	return m
}

// fakeHome points $HOME at a temp dir with a ~/.claude/plans folder, so the
// launch dirs and the plans folder are the test's own.
func fakeHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	mustMkdirAll(t, filepath.Join(home, ".claude", "plans"))
	return home
}

func mustMkdirAll(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
}

func mustWrite(t *testing.T, path, body string) {
	t.Helper()
	mustMkdirAll(t, filepath.Dir(path))
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// seedFor builds the seed prompt exactly as assistantCmd would.
func seedFor(t *testing.T, m Model) string {
	t.Helper()
	cwd, memDir, projDir, unresolved := m.assistantContext()
	return m.buildSeedPrompt(claudeAssistant(t), cwd, projDir, memDir, unresolved)
}

// Item 5: a rule file inside the project that is a symlink to a file outside
// it is not editable by a session opened in the project, so the prompt must not
// promise the edit. The old check compared path strings and promised it.
func TestSeedPromptSymlinkedRuleFileIsOutside(t *testing.T) {
	fakeHome(t)
	proj, elsewhere := t.TempDir(), t.TempDir()
	mustWrite(t, filepath.Join(elsewhere, "AGENTS.md"), "# shared rules\n")
	link := filepath.Join(proj, "AGENTS.md")
	if err := os.Symlink(filepath.Join(elsewhere, "AGENTS.md"), link); err != nil {
		t.Fatal(err)
	}
	m := filesModel(t, memory.DocFile{
		Path: link, Title: "AGENTS.md", Body: "# shared rules\n", Kind: memory.DocRules,
		Provider: memory.ProviderAgents, Scope: "widgets", ProjectName: "widgets", ProjectDir: proj,
	})
	out := seedFor(t, m)
	if s := seedLine(out, "Selected file:"); !strings.Contains(s, "outside the folders") {
		t.Errorf("selected-file line = %q, want it to say the symlink target is outside the session", s)
	}
	if strings.Contains(out, "edit the selected file") {
		t.Errorf("prompt promises an edit through a symlink that leaves the session:\n%s", out)
	}
}

// Item 3: the capability line offers only what the session can reach. A
// project launch with no memory dir has no memories, and plans sit outside
// any project; a global launch in ~/.claude reaches plans but no memory dir.
func TestSeedPromptCapabilitiesMatchTheSession(t *testing.T) {
	home := fakeHome(t)

	proj := t.TempDir()
	projDoc := filesModel(t, memory.DocFile{
		Path: filepath.Join(proj, "AGENTS.md"), Title: "AGENTS.md", Body: "# rules\n", Kind: memory.DocRules,
		Provider: memory.ProviderAgents, Scope: "widgets", ProjectName: "widgets", ProjectDir: proj,
	})
	mustWrite(t, filepath.Join(proj, "AGENTS.md"), "# rules\n")
	line := seedLine(seedFor(t, projDoc), "You can")
	if !strings.Contains(line, "edit the selected file") {
		t.Errorf("project doc: capability line %q should offer editing the file", line)
	}
	if strings.Contains(line, "memor") || strings.Contains(line, "plan") {
		t.Errorf("project doc with no memory dir: capability line %q offers memories or plans it can't reach", line)
	}

	global := filepath.Join(home, ".claude", "CLAUDE.md")
	mustWrite(t, global, "# global\n")
	globalDoc := filesModel(t, memory.DocFile{Path: global, Title: "CLAUDE.md", Body: "# global\n", Kind: memory.DocRules, Scope: "global"})
	line = seedLine(seedFor(t, globalDoc), "You can")
	if !strings.Contains(line, "plan") {
		t.Errorf("global launch in ~/.claude: capability line %q should offer plans", line)
	}
	if strings.Contains(line, "memor") {
		t.Errorf("global launch has no memory dir: capability line %q offers memories", line)
	}

	memDir := filepath.Join(t.TempDir(), "memory")
	mustMkdirAll(t, memDir)
	mem := modelWithMemory(t, t.TempDir(), memDir)
	line = seedLine(seedFor(t, mem), "You can")
	if !strings.Contains(line, "memor") {
		t.Errorf("memory launch: capability line %q should offer memory work", line)
	}
	if strings.Contains(line, "plan") {
		t.Errorf("project launch: capability line %q offers plans outside the project", line)
	}
}

// Item 3: the "may be misfiled" note is about relocating memories, which pulls
// a /files session away from the file it was opened for. Only the memory
// sources get it.
func TestSeedPromptFilesSkipsTheMisfiledNote(t *testing.T) {
	fakeHome(t)
	memDir := driftDir(t)
	gone := filepath.Join(t.TempDir(), "gone")
	m := filesModel(t, memory.DocFile{
		Path: filepath.Join(memDir, "MEMORY.md"), Title: "MEMORY.md", Body: "# index\n", Kind: memory.DocIndex,
		Scope: "gone", ProjectName: "gone", ProjectDir: gone, MemoryDir: memDir,
	})
	cwd, md, pd, unresolved := m.assistantContext()
	if !unresolved {
		t.Fatal("setup: want an unresolved project")
	}
	if out := m.buildSeedPrompt(claudeAssistant(t), cwd, pd, md, unresolved); strings.Contains(out, "couldn't resolve") {
		t.Errorf("a /files launch carries the relocate-memories note:\n%s", out)
	}
}

// Item 2: a scan-root project deleted after discovery has no memory dir and no
// folder; @ used to launch in ~/.claude/projects with a prompt naming a project
// and file that no longer exist. It now refuses and says why.
func TestHandoffRefusesADeletedProject(t *testing.T) {
	fakeHome(t)
	gone := filepath.Join(t.TempDir(), "gone")
	m := filesModel(t, memory.DocFile{
		Path: filepath.Join(gone, "AGENTS.md"), Title: "AGENTS.md", Body: "# rules\n", Kind: memory.DocRules,
		Provider: memory.ProviderAgents, Scope: "gone", ProjectName: "gone", ProjectDir: gone,
	})
	if cmd := m.assistantCmd("claude"); cmd == nil {
		t.Fatal("want a status command, got nil")
	}
	if !strings.Contains(m.status, "no longer exists") || !strings.Contains(m.status, gone) {
		t.Errorf("status = %q, want it to name %s and say it no longer exists", m.status, gone)
	}
}

// Item 4: the read-only hint for a /files row an assistant session can't
// write (the vendor global files, outside every folder engram opens) must not
// read as if @ will edit it.
func TestFilesHintForRowsAnAssistantCannotWrite(t *testing.T) {
	home := fakeHome(t)
	gemini := filepath.Join(home, ".gemini", "GEMINI.md")
	mustWrite(t, gemini, "# gemini\n")
	press := func(m Model) string {
		tm, _ := tea.Model(m).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
		return tm.(Model).status
	}
	outside := filesModel(t, memory.DocFile{
		Path: gemini, Title: "GEMINI.md", Body: "# gemini\n", Kind: memory.DocRules,
		Provider: memory.ProviderGemini, Scope: "global",
	})
	if got := press(outside); got != readOnlyOutsideHint {
		t.Errorf("hint on ~/.gemini/GEMINI.md = %q, want %q", got, readOnlyOutsideHint)
	}

	proj := t.TempDir()
	mustWrite(t, filepath.Join(proj, "AGENTS.md"), "# rules\n")
	inside := filesModel(t, memory.DocFile{
		Path: filepath.Join(proj, "AGENTS.md"), Title: "AGENTS.md", Body: "# rules\n", Kind: memory.DocRules,
		Provider: memory.ProviderAgents, Scope: "widgets", ProjectName: "widgets", ProjectDir: proj,
	})
	if got := press(inside); got != readOnlyHint[srcFiles] {
		t.Errorf("hint on a project rule file = %q, want %q", got, readOnlyHint[srcFiles])
	}
}

// Item 5: the folders the prompt treats as writable are the ones the command
// line grants: the launch dir, plus the memory dir only when it sits outside.
func TestSessionDirsMatchTheGrant(t *testing.T) {
	cases := []struct {
		cwd, memDir string
		want        []string
	}{
		{"/p", "", []string{"/p"}},
		{"/p", "/p/mem", []string{"/p"}},
		{"/p", "/h/.claude/projects/x/memory", []string{"/p", "/h/.claude/projects/x/memory"}},
	}
	for _, c := range cases {
		got := sessionDirs(c.cwd, c.memDir)
		if strings.Join(got, "|") != strings.Join(c.want, "|") {
			t.Errorf("sessionDirs(%q, %q) = %v, want %v", c.cwd, c.memDir, got, c.want)
		}
		if add := sessionAddDir(c.cwd, c.memDir); (add != "") != (len(c.want) == 2) {
			t.Errorf("sessionAddDir(%q, %q) = %q disagrees with sessionDirs %v", c.cwd, c.memDir, add, got)
		}
	}
}

// Item 5, the dangling case: a rule file that links out of the project to a
// target not written yet is outside the session too.
func TestSeedPromptDanglingSymlinkIsOutside(t *testing.T) {
	fakeHome(t)
	proj := t.TempDir()
	link := filepath.Join(proj, "AGENTS.md")
	if err := os.Symlink(filepath.Join(t.TempDir(), "not-yet", "AGENTS.md"), link); err != nil {
		t.Fatal(err)
	}
	m := filesModel(t, memory.DocFile{
		Path: link, Title: "AGENTS.md", Body: "", Kind: memory.DocRules,
		Provider: memory.ProviderAgents, Scope: "widgets", ProjectName: "widgets", ProjectDir: proj,
	})
	if out := seedFor(t, m); strings.Contains(out, "edit the selected file") {
		t.Errorf("prompt promises an edit through a dangling link out of the project:\n%s", out)
	}
}

// Item 3, nothing in reach: a project doc the session can't write, with no
// memory dir and no plans, gets no capability list at all, and its scope names
// only the selected file.
func TestSeedPromptWithNothingInReach(t *testing.T) {
	fakeHome(t)
	proj, elsewhere := t.TempDir(), t.TempDir()
	mustWrite(t, filepath.Join(elsewhere, "AGENTS.md"), "# rules\n")
	link := filepath.Join(proj, "AGENTS.md")
	if err := os.Symlink(filepath.Join(elsewhere, "AGENTS.md"), link); err != nil {
		t.Fatal(err)
	}
	m := filesModel(t, memory.DocFile{
		Path: link, Title: "AGENTS.md", Body: "# rules\n", Kind: memory.DocRules,
		Provider: memory.ProviderAgents, Scope: "widgets", ProjectName: "widgets", ProjectDir: proj,
	})
	out := seedFor(t, m)
	if seedLine(out, "You can") != "" {
		t.Errorf("a session with nothing in reach still gets a capability list:\n%s", out)
	}
	if !strings.Contains(out, "hold no memories or plans") {
		t.Errorf("want the nothing-in-reach line:\n%s", out)
	}
	if s := seedLine(out, "Scope:"); strings.Contains(s, "memory files") || strings.Contains(s, "plans") {
		t.Errorf("scope %q names memories or plans the session can't reach", s)
	}
}

// Item 4, the deleted project: its row's hint is the launch's own refusal,
// not "@ can still read it".
func TestFilesHintOnADeletedProjectIsTheRefusal(t *testing.T) {
	fakeHome(t)
	gone := filepath.Join(t.TempDir(), "gone")
	m := filesModel(t, memory.DocFile{
		Path: filepath.Join(gone, "AGENTS.md"), Title: "AGENTS.md", Body: "# rules\n", Kind: memory.DocRules,
		Provider: memory.ProviderAgents, Scope: "gone", ProjectName: "gone", ProjectDir: gone,
	})
	tm, _ := tea.Model(m).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	if got := tm.(Model).status; !strings.Contains(got, "no longer exists") {
		t.Errorf("hint on a deleted project's row = %q, want the refusal", got)
	}
}
