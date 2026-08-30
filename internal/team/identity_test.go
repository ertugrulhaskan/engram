package team

import (
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// newRepo makes a repository in a temp dir with the given origin URL ("" for
// none), with the developer's own git config kept out of the way — a
// url.<base>.insteadOf rewrite would otherwise change what get-url reports.
func newRepo(t *testing.T, origin string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
	dir := t.TempDir()
	gitT(t, dir, "init", "-q")
	if origin != "" {
		gitT(t, dir, "remote", "add", "origin", origin)
	}
	return dir
}

// ResolveKey is the fallback rule promote and pull share: the remote wins; the
// alias stands in for a directory with no remote and for one that has gone;
// nothing stands in when git could not say; a malformed alias never becomes a
// key.
func TestResolveKey(t *testing.T) {
	plain := t.TempDir()
	cases := []struct {
		name  string
		dir   string
		alias string
		want  string
	}{
		{"remote wins over an alias", newRepo(t, "git@github.com:acme/app.git"), "acme-app", "github.com/acme/app"},
		{"plain directory, alias", plain, "Acme-App", "alias/acme-app"},
		{"repository without origin, alias", newRepo(t, ""), "acme-app", "alias/acme-app"},
		{"plain directory, no alias", plain, "", ""},
		{"vanished directory keeps its alias", filepath.Join(plain, "gone"), "acme-app", "alias/acme-app"},
		{"vanished directory, no alias", filepath.Join(plain, "gone"), "", ""},
		{"unkeyable origin: git couldn't say", newRepo(t, "/srv/repo.git"), "acme-app", ""},
		{"malformed alias never becomes a key", plain, "bad/alias", ""},
	}
	for _, c := range cases {
		if got, _ := ResolveKey(c.dir, c.alias); got != c.want {
			t.Errorf("%s: ResolveKey = %q, want %q", c.name, got, c.want)
		}
	}
	if _, st := ResolveKey(newRepo(t, "/srv/repo.git"), "acme-app"); st != RemoteUnknown {
		t.Errorf("ResolveKey(unkeyable) state = %v, want RemoteUnknown", st)
	}
	// The home folder is never keyed, whatever the config says.
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		if key, _ := ResolveKey(home, "personal"); key != "" {
			t.Errorf("ResolveKey(home, alias) = %q, want \"\"", key)
		}
		if !IsHomeDir(home) || !IsHomeDir(home+string(os.PathSeparator)) || IsHomeDir(t.TempDir()) {
			t.Error("IsHomeDir: home (with and without a trailing separator) must match; a temp dir must not")
		}
	}
	if _, st, _ := ClassifyRemote(filepath.Join(plain, "gone")); st != RemoteGone {
		t.Errorf("ClassifyRemote(vanished) state = %v, want RemoteGone", st)
	}
	if _, st, _ := ClassifyRemote(newRepo(t, "/srv/repo.git")); st != RemoteUnknown {
		t.Errorf("ClassifyRemote(unkeyable origin) state = %v, want RemoteUnknown", st)
	}
}

func TestProjectKey(t *testing.T) {
	dir := newRepo(t, "git@github.com:Acme/App.git")
	got, err := ProjectKey(dir)
	if err != nil {
		t.Fatalf("ProjectKey: %v", err)
	}
	if want := "github.com/acme/app"; got != want {
		t.Errorf("ProjectKey = %q, want %q", got, want)
	}
}

func TestProjectKeyEmptyDir(t *testing.T) {
	if _, err := ProjectKey(""); err == nil {
		t.Error("expected an error for an empty project dir")
	}
}

// ErrNoRemote is the one ProjectKey failure an alias may stand in for: a plain
// directory, or a repository without origin. A missing directory and an origin
// whose URL has no host/path to key by are different failures — git could not
// say — and must not read as "no remote".
func TestProjectKeyNoRemote(t *testing.T) {
	plain := t.TempDir()
	nested := filepath.Join(newRepo(t, ""), "sub")
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name     string
		dir      string
		noRemote bool
	}{
		{"plain directory", plain, true},
		{"repository without origin", newRepo(t, ""), true},
		{"nested plain dir inside a repo without origin", nested, true},
		{"missing directory", filepath.Join(plain, "gone"), false},
		{"origin with no host/path", newRepo(t, "/srv/repo.git"), false},
	}
	for _, c := range cases {
		_, err := ProjectKey(c.dir)
		if err == nil {
			t.Errorf("%s: ProjectKey succeeded, want an error", c.name)
			continue
		}
		if got := errors.Is(err, ErrNoRemote); got != c.noRemote {
			t.Errorf("%s: errors.Is(ErrNoRemote) = %v, want %v (err: %v)", c.name, got, c.noRemote, err)
		}
	}
	if key, err := ProjectKey(newRepo(t, "git@github.com:acme/app.git")); err != nil || key != "github.com/acme/app" {
		t.Errorf("repository with origin: key=%q err=%v", key, err)
	}
	// A missing directory is reported as the stat error, so a caller can tell
	// "gone" apart from every other way git can fail to say.
	if _, err := ProjectKey(filepath.Join(plain, "gone")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("missing directory: err = %v, want one wrapping fs.ErrNotExist", err)
	}
}
