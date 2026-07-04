package team

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/ertugrulhaskan/engram/internal/memory"
)

// TestContainsSymlink covers the guard that keeps promote/pull from acting
// through a symlink a teammate committed into the store.
func TestContainsSymlink(t *testing.T) {
	root := t.TempDir()
	deep := filepath.Join(root, "a", "b", "c.md")
	if err := os.MkdirAll(filepath.Dir(deep), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(deep, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if containsSymlink(root, deep) {
		t.Error("a plain nested path must not be flagged")
	}
	if containsSymlink(root, filepath.Join(root, "a", "b", "new.md")) {
		t.Error("a not-yet-existing leaf under real dirs must not be flagged")
	}

	link := filepath.Join(root, "a", "b", "evil.md")
	if err := os.Symlink("/etc/hosts", link); err != nil {
		t.Skip("symlinks unsupported here")
	}
	if !containsSymlink(root, link) {
		t.Error("a symlinked leaf must be flagged")
	}
	linkDir := filepath.Join(root, "a", "sub")
	if err := os.Symlink(root, linkDir); err != nil {
		t.Fatal(err)
	}
	if !containsSymlink(root, filepath.Join(linkDir, "target.md")) {
		t.Error("a symlinked intermediate directory must be flagged")
	}
	if containsSymlink(root, filepath.Join(t.TempDir(), "other.md")) {
		t.Error("a path outside root must not be flagged")
	}
}

// TestInitBlocksExtTransport proves engram's clone refuses git's `ext::`
// command-execution transport even when the ambient git config enables it.
func TestInitBlocksExtTransport(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	root := t.TempDir()
	hermeticGitEnv(t, root)
	// Enable ext in config, so a pass proves engram's own -c flag wins — not git's
	// default (which already blocks ext on modern versions).
	cfg := filepath.Join(root, "gitconfig")
	if err := os.WriteFile(cfg, []byte("[user]\n\tname = U\n\temail = u@example.com\n[protocol \"ext\"]\n\tallow = always\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", cfg)

	poc := filepath.Join(root, "pwned")
	if err := InitTeam("ext::sh -c touch% " + poc); err == nil {
		t.Error("InitTeam must fail on an ext:: transport URL")
	}
	if _, err := os.Stat(poc); !os.IsNotExist(err) {
		t.Errorf("ext:: command executed — the RCE guard failed (poc exists): %v", err)
	}
}

// TestPromoteRefusesSymlink proves promote won't write *through* a symlink
// planted at the store destination (an arbitrary-file overwrite otherwise).
func TestPromoteRefusesSymlink(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	root := t.TempDir()
	hermeticGitEnv(t, root)
	bare := filepath.Join(root, "remote.git")
	gitT(t, "", "init", "--bare", bare)
	if err := InitTeam("file://" + bare); err != nil {
		t.Fatalf("InitTeam: %v", err)
	}
	teamDir, _ := Dir()
	key := "github.com/acme/app"

	outside := filepath.Join(root, "outside.txt")
	if err := os.WriteFile(outside, []byte("ORIGINAL"), 0o644); err != nil {
		t.Fatal(err)
	}
	storeDest := filepath.Join(teamDir, "projects", filepath.FromSlash(key), "note.md")
	if err := os.MkdirAll(filepath.Dir(storeDest), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, storeDest); err != nil {
		t.Skip("symlinks unsupported here")
	}

	memDir := filepath.Join(root, "proj", "memory")
	if err := os.MkdirAll(memDir, 0o755); err != nil {
		t.Fatal(err)
	}
	memPath := filepath.Join(memDir, "note.md")
	if err := os.WriteFile(memPath, []byte("---\nname: note\n---\n# Note\n\nsecret body\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := Promote(memPath, key); err == nil {
		t.Error("promote through a symlink must be refused")
	}
	if b, _ := os.ReadFile(outside); string(b) != "ORIGINAL" {
		t.Errorf("promote wrote through the symlink — target overwritten: %q", b)
	}
}

// TestPullSkipsSymlinkEntry proves pull never reads *through* a symlinked store
// entry (which could point at a private file outside the store).
func TestPullSkipsSymlinkEntry(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	root := t.TempDir()
	hermeticGitEnv(t, root)
	bare := filepath.Join(root, "remote.git")
	gitT(t, "", "init", "--bare", bare)
	if err := InitTeam("file://" + bare); err != nil {
		t.Fatalf("InitTeam: %v", err)
	}
	teamDir, _ := Dir()
	key := "github.com/acme/app"

	secret := filepath.Join(root, "secret.txt")
	if err := os.WriteFile(secret, []byte("PRIVATE-KEY-MATERIAL"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(teamDir, "projects", filepath.FromSlash(key), "evil.md")
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, link); err != nil {
		t.Skip("symlinks unsupported here")
	}

	memDir := filepath.Join(root, "proj", "memory")
	if err := os.MkdirAll(memDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Pull([]ProjectTarget{{Key: key, MemoryDir: memDir}}); err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if _, err := os.Stat(filepath.Join(memDir, "evil.md")); !os.IsNotExist(err) {
		t.Error("pull copied a symlinked store entry into the memory dir")
	}
}

// TestPullWithdrawalKeepsLocalEdits proves withdrawal propagation removes a clean
// (anchor-matching) copy but never a copy the user edited since it last synced.
func TestPullWithdrawalKeepsLocalEdits(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	root := t.TempDir()
	hermeticGitEnv(t, root)
	bare := filepath.Join(root, "remote.git")
	gitT(t, "", "init", "--bare", bare)
	if err := InitTeam("file://" + bare); err != nil {
		t.Fatalf("InitTeam: %v", err)
	}
	teamDir, _ := Dir()
	key := "github.com/acme/app"
	memDir := filepath.Join(root, "proj", "memory")
	if err := os.MkdirAll(memDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// A team memory the user edited locally: its recorded anchor does NOT match
	// its current content, so withdrawal must not delete it.
	edited := "---\nname: m\nengram:\n    id: ED-1\n    scope: team\n    project: " + key +
		"\n    syncedHash: STALEANCHOR\n---\n# M\n\nEDITED locally, never pushed\n"
	editedPath := filepath.Join(memDir, "edited.md")
	if err := os.WriteFile(editedPath, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}

	// A clean team memory whose anchor matches its content: withdrawal removes it.
	cleanRaw := func(hash string) string {
		return "---\nname: c\nengram:\n    id: CL-1\n    scope: team\n    project: " + key +
			"\n    syncedHash: " + hash + "\n---\n# C\n\nclean body\n"
	}
	dig, err := memory.ContentDigest(cleanRaw("x")) // the digest excludes the engram block
	if err != nil {
		t.Fatal(err)
	}
	cleanPath := filepath.Join(memDir, "clean.md")
	if err := os.WriteFile(cleanPath, []byte(cleanRaw(dig)), 0o644); err != nil {
		t.Fatal(err)
	}

	addWithdrawn(teamDir, "ED-1", "edited.md")
	addWithdrawn(teamDir, "CL-1", "clean.md")

	res, err := Pull([]ProjectTarget{{Key: key, MemoryDir: memDir}})
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if _, err := os.Stat(editedPath); err != nil {
		t.Error("a locally-edited team memory must NOT be deleted by withdrawal propagation")
	}
	if _, err := os.Stat(cleanPath); !os.IsNotExist(err) {
		t.Error("a clean (anchor-matching) withdrawn team memory should be removed")
	}
	if res.Removed != 1 {
		t.Errorf("Removed = %d, want 1 (only the clean copy)", res.Removed)
	}
}
