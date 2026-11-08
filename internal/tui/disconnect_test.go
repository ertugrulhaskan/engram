package tui

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/ertugrulhaskan/engram/internal/config"
	"github.com/ertugrulhaskan/engram/internal/memory"
	"github.com/ertugrulhaskan/engram/internal/team"
)

// storeFixture stands up a hermetic team store with a bare remote under a temp
// config dir, and returns its remote URL.
func storeFixture(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "xdg"))
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
	for k, v := range map[string]string{"GIT_AUTHOR_NAME": "t", "GIT_AUTHOR_EMAIL": "t@example.com", "GIT_COMMITTER_NAME": "t", "GIT_COMMITTER_EMAIL": "t@example.com"} {
		t.Setenv(k, v)
	}
	bare := filepath.Join(root, "remote.git")
	if out, err := exec.Command("git", "init", "--bare", bare).CombinedOutput(); err != nil {
		t.Fatalf("git init --bare: %v\n%s", err, out)
	}
	if err := team.InitTeam("file://" + bare); err != nil {
		t.Fatalf("InitTeam: %v", err)
	}
	return "file://" + bare
}

// The palette's init row names the connected store, and >disconnect asks
// first, naming the store and how many shared memories stay as personal.
func TestDisconnectFromThePalette(t *testing.T) {
	url := storeFixture(t)
	shared := memory.Memory{Title: "s", Body: "# s\n", Path: "/x/s.md", Shared: memory.EngramMeta{ID: "1", Scope: "team"}}
	mine := memory.Memory{Title: "p", Body: "# p\n", Path: "/x/p.md"}
	var tm tea.Model = New([]memory.Memory{shared, mine}, nil, nil, config.Config{})
	tm, _ = tm.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m := tm.(Model)
	if m.storeURL != url {
		t.Fatalf("storeURL = %q, want %q", m.storeURL, url)
	}
	if sub := m.initSub(); !strings.Contains(sub, url) {
		t.Errorf("init row = %q, want it to name the connected store", sub)
	}

	tm = switchVia(tm, ">disconnect")
	m = tm.(Model)
	if m.mode != modeDisconnectConfirm {
		t.Fatalf("mode = %v after >disconnect, want the confirm", m.mode)
	}
	if len(m.disconnectPaths) != 1 || m.disconnectPaths[0] != shared.Path {
		t.Errorf("disconnectPaths = %v, want only the team memory", m.disconnectPaths)
	}
	box := ansi.Strip(m.disconnectModal())
	// The temp-dir URL is longer than the dialog: its end, the repo name, must
	// still show.
	for _, want := range []string{"/remote.git", "1 shared memory stays", "Nothing is deleted"} {
		if !strings.Contains(box, want) {
			t.Errorf("confirm is missing %q:\n%s", want, box)
		}
	}
	for i, ln := range strings.Split(box, "\n") {
		if w := lipgloss.Width(ln); w != m.boxWidth()+2 {
			t.Errorf("confirm line %d is %d cells, want %d", i, w, m.boxWidth()+2)
		}
	}

	tm, _ = tm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	if got := tm.(Model); got.mode != modeNormal || !team.IsInitialized() {
		t.Error("n must cancel and leave the store connected")
	}
}

// With no store, >disconnect says so instead of opening a confirm.
func TestDisconnectWithNoStore(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	tm := switchVia(ready(t), ">disconnect")
	m := tm.(Model)
	if m.mode == modeDisconnectConfirm {
		t.Error("opened a disconnect confirm with no store connected")
	}
	if !strings.Contains(m.status, "no team store") && !strings.Contains(m.status, "git") {
		t.Errorf("status = %q, want it to say no store is connected", m.status)
	}
	if sub := m.initSub(); strings.Contains(sub, "connected to") {
		t.Errorf("init row = %q with no store", sub)
	}
}

// tailFit keeps the end of a long URL and fits it exactly.
func TestTailFit(t *testing.T) {
	long := "https://gitlab.example.com/platform/infrastructure/shared/team-memories-store.git"
	got := tailFit(long, 40)
	if lipgloss.Width(got) != 40 || !strings.HasPrefix(got, "…") || !strings.HasSuffix(got, "team-memories-store.git") {
		t.Errorf("tailFit = %q (%d cells), want 40 cells ending in the repo name", got, lipgloss.Width(got))
	}
	if short := "git@github.com:acme/m.git"; tailFit(short, 40) != short {
		t.Error("tailFit changed a URL that fits")
	}
}
