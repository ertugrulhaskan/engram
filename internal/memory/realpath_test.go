package memory

import (
	"os"
	"path/filepath"
	"testing"
)

// RealPath resolves symlinks, and for a path that doesn't exist yet resolves
// its nearest existing parent, so it still compares equal to that folder.
func TestRealPath(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(base, "target")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	cases := map[string]string{
		link:                                  target,
		filepath.Join(link, "new.md"):         filepath.Join(target, "new.md"),
		filepath.Join(link, "a", "b", "c.md"): filepath.Join(target, "a", "b", "c.md"),
		target + string(os.PathSeparator):     target,
		"":                                    "",
	}
	for in, want := range cases {
		if got := RealPath(in); got != want {
			t.Errorf("RealPath(%q) = %q, want %q", in, got, want)
		}
	}
}

// A dangling symlink is followed to where it points, so a link out of a folder
// whose target doesn't exist yet still resolves outside that folder.
func TestRealPathFollowsADanglingLink(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	proj := filepath.Join(base, "proj")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(proj, "AGENTS.md")
	if err := os.Symlink("../shared/AGENTS.md", link); err != nil {
		t.Fatal(err)
	}
	if got, want := RealPath(link), filepath.Join(base, "shared", "AGENTS.md"); got != want {
		t.Errorf("RealPath(dangling link) = %q, want its target %q", got, want)
	}
	loop := filepath.Join(base, "loop")
	if err := os.Symlink(loop, loop); err != nil {
		t.Fatal(err)
	}
	_ = RealPath(loop) // must return, not recurse forever
}

// A relative dangling link inside a symlinked folder resolves against the
// folder's real location, so its ".." climbs out of the real folder.
func TestRealPathDanglingLinkUnderASymlinkedFolder(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	deep := filepath.Join(base, "outside", "deep")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(base, "proj"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(deep, filepath.Join(base, "proj", "sub")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../y", filepath.Join(deep, "x")); err != nil {
		t.Fatal(err)
	}
	if got, want := RealPath(filepath.Join(base, "proj", "sub", "x")), filepath.Join(base, "outside", "y"); got != want {
		t.Errorf("RealPath = %q, want %q", got, want)
	}
}
