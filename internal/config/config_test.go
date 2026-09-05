package config

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestScanDefaults(t *testing.T) {
	var c Config // zero value = keys absent
	if got := c.ScanAction(); got != "block" {
		t.Errorf("default ScanAction = %q, want block", got)
	}
	if c.ScanPII() {
		t.Error("default ScanPII should be false (secrets only)")
	}
	if got := (Config{SecretScanAction: "bogus"}).ScanAction(); got != "block" {
		t.Errorf("unrecognized action should fall back to block, got %q", got)
	}
	for _, a := range []string{"block-strict", "warn", "off"} {
		if got := (Config{SecretScanAction: a}).ScanAction(); got != a {
			t.Errorf("ScanAction(%q) = %q", a, got)
		}
	}
	if !(Config{SecretScanScope: "secrets+pii"}).ScanPII() {
		t.Error("secrets+pii should enable PII scanning")
	}
}

func TestDir(t *testing.T) {
	base := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", base)

	got, err := Dir()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(base, "engram"); got != want {
		t.Errorf("Dir() = %q, want %q", got, want)
	}
}

func TestRoundTrip(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	// Absent file → zero Config.
	if c := load(); c.Theme != "" || c.Editor != "" {
		t.Fatalf("missing config should be zero, got %+v", c)
	}

	// ScanRoots included: it is a slice, so Config is no longer comparable with
	// != — reflect.DeepEqual is now the round-trip check, and it also proves the
	// slice survives the JSON round trip rather than coming back nil.
	want := Config{Theme: "crt", Editor: "code --wait", ScanRoots: []string{"~/code", "/srv/work"}}
	if err := Save(want); err != nil {
		t.Fatal(err)
	}
	if got := load(); !reflect.DeepEqual(got, want) {
		t.Errorf("round trip: got %+v, want %+v", got, want)
	}

	// An omitted scanRoots must load as nil, not an empty non-nil slice, so a
	// config written before this field existed still round-trips unchanged.
	bare := Config{Theme: "midnight"}
	if err := Save(bare); err != nil {
		t.Fatal(err)
	}
	if got := load(); got.ScanRoots != nil {
		t.Errorf("absent scanRoots loaded as %#v, want nil", got.ScanRoots)
	}
}

// load is Read with the error dropped. Tests that assert on a written value
// have already asserted that the write succeeded, so the error adds nothing —
// and Read is deliberately the only reader outside them, so that production
// code has to decide what an unreadable config means.
func load() Config {
	c, _ := Read()
	return c
}

// Save must not be able to leave a file engram then refuses to launch from.
// os.WriteFile truncates before it writes, and Read now reports a half-written
// file as ErrUnparseable, which main.go treats as fatal — so the write goes to
// a temp file beside the target and is renamed over it.
//
// The inode is what proves it. A rename replaces the directory entry, so the
// file that answers to the config path afterwards is a different one; a
// truncate-and-write keeps the same inode and is the form that can be caught
// half-done. A torn write itself can't be forced from a test, so this pins the
// mechanism that makes it impossible instead.
//
// The trade-off is deliberate and worth stating: this needs write permission on
// the *directory*, which os.WriteFile did not once the file existed. A
// read-only config dir holding a writable config.json now fails to save rather
// than saving non-atomically. That is the rarer situation, and failing loudly
// there beats a torn file that stops engram from starting.
func TestSaveReplacesTheFileRatherThanTruncatingIt(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	if err := Save(Config{Theme: "crt"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	p, err := Path()
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	// os.SameFile compares the platform's own file identity — inode+device on
	// Unix, the file index on Windows — without naming either, so this builds
	// everywhere engram ships a binary. (syscall.Stat_t does not exist on
	// Windows, and a runtime t.Skip cannot rescue a compile error.)
	stat := func() os.FileInfo {
		st, err := os.Stat(p)
		if err != nil {
			t.Fatalf("Stat: %v", err)
		}
		return st
	}
	before := stat()
	if err := Save(Config{Theme: "paperback"}); err != nil {
		t.Fatalf("second Save: %v", err)
	}
	if after := stat(); os.SameFile(before, after) {
		t.Error("the config is the same file after a Save — it was truncated in place, not renamed over")
	}
	if got := load(); got.Theme != "paperback" {
		t.Errorf("theme = %q, want the second write", got.Theme)
	}
	// Nothing left behind, and the mode the file has always had rather than
	// CreateTemp's 0600.
	entries, err := os.ReadDir(filepath.Dir(p))
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != filepath.Base(p) {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("config dir holds %v, want only %q", names, filepath.Base(p))
	}
}

// A rename replaces the file, so the mode has to be carried across
// deliberately: os.WriteFile applied its perm only when *creating*, leaving a
// mode the user had set alone on every later write. Losing that would silently
// widen a config holding the editor command, scan roots and project aliases.
func TestSavePreservesTheFileMode(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	if err := Save(Config{Theme: "crt"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	p, err := Path()
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	if err := os.Chmod(p, 0o600); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	if err := Save(Config{Theme: "paperback"}); err != nil {
		t.Fatalf("second Save: %v", err)
	}
	st, err := os.Stat(p)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if got := st.Mode().Perm(); got != 0o600 {
		t.Errorf("config mode = %v after a Save, want the 0600 the user set", got)
	}
}

// A config.json that a dotfiles manager symlinks into a repo must keep
// receiving engram's writes. os.WriteFile wrote *through* the link; a bare
// rename would replace the link with a regular file and silently strand the
// repo copy, so Save resolves the link first and renames onto the real file.
func TestSaveFollowsASymlinkedConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	p, err := Path()
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	// The "dotfiles repo" copy, and the link engram is pointed at.
	repo := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(repo, []byte(`{"theme":"crt"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(repo, p); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := Save(Config{Theme: "paperback"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	st, err := os.Lstat(p)
	if err != nil {
		t.Fatalf("Lstat: %v", err)
	}
	if st.Mode()&os.ModeSymlink == 0 {
		t.Error("Save replaced the symlink with a regular file — the dotfiles copy is stranded")
	}
	got, err := os.ReadFile(repo)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(got), "paperback") {
		t.Errorf("the linked-to file was not written: %s", got)
	}
	// And nothing was left beside either copy.
	for _, dir := range []string{filepath.Dir(p), filepath.Dir(repo)} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 1 {
			t.Errorf("%s holds %d entries, want 1", dir, len(entries))
		}
	}
}

// The dotfiles bootstrap order: the link is made before the repo copy exists.
// EvalSymlinks fails on a dangling link, and falling back to the link's own
// path would have the rename eat the link — so Save follows it by hand and
// creates the target, which is what os.WriteFile did.
func TestSaveFollowsADanglingSymlink(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	p, err := Path()
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(t.TempDir(), "config.json") // deliberately not created
	if err := os.Symlink(repo, p); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := Save(Config{Theme: "paperback"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	st, err := os.Lstat(p)
	if err != nil {
		t.Fatalf("Lstat: %v", err)
	}
	if st.Mode()&os.ModeSymlink == 0 {
		t.Fatal("Save replaced the dangling symlink with a regular file — the link is gone for good")
	}
	got, err := os.ReadFile(repo)
	if err != nil {
		t.Fatalf("the link target was never created: %v", err)
	}
	if !strings.Contains(string(got), "paperback") {
		t.Errorf("link target = %s, want the saved theme", got)
	}
}

// TestReadTreatsAnEmptyFileAsAbsent pins the one unparseable state that carries
// nothing to protect. The truncating write before v0.5.0 could leave a zero-byte
// config.json behind (a session killed between the truncate and the write, or a
// full disk); if Read reported that as ErrUnparseable, main.go would refuse to
// start the upgraded binary, and the repair it advertises (/settings) lives
// inside the TUI that would not open. Whitespace-only counts as empty too, and
// the one write path may replace either.
func TestReadTreatsAnEmptyFileAsAbsent(t *testing.T) {
	for _, body := range []string{"", "\n \t\n"} {
		t.Setenv("XDG_CONFIG_HOME", t.TempDir())
		p, err := Path()
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		c, err := Read()
		if err != nil {
			t.Fatalf("Read() over %q = %v; want nil: an empty file is absent, not unparseable", body, err)
		}
		if !reflect.DeepEqual(c, Config{}) {
			t.Errorf("Read() over %q = %+v; want the zero Config", body, c)
		}
		if err := Update(func(c *Config) error { c.Theme = "crt"; return nil }); err != nil {
			t.Fatalf("Update over %q = %v; want success, there is nothing in it to protect", body, err)
		}
		if got := load(); got.Theme != "crt" {
			t.Errorf("after Update over %q, Theme = %q; want crt", body, got.Theme)
		}
	}
}

// TestSaveRefusesAReadOnlyConfig pins the guard os.WriteFile gave for free and
// the atomic write has to ask for by hand: a rename replaces the file without
// opening it, so without the probe a chmod 444 config was rewritten in place
// and re-chmodded, with nothing to show it had happened.
func TestSaveRefusesAReadOnlyConfig(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root writes through any mode")
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := Save(Config{Theme: "crt"}); err != nil {
		t.Fatal(err)
	}
	p, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(p, 0o644) }) // so the temp dir is removable on every platform
	if err := os.Chmod(p, 0o444); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := Save(Config{Theme: "dark"}); !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("Save over a read-only config = %v; want a permission error", err)
	}
	after, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Errorf("read-only config was rewritten:\n%s", after)
	}
	st, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o444 {
		t.Errorf("mode after the refusal = %v; want 444", st.Mode().Perm())
	}
	entries, err := os.ReadDir(filepath.Dir(p))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != filepath.Base(p) {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("config dir holds %v after a refused write, want only %q", names, filepath.Base(p))
	}
}
