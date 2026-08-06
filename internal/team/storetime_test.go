package team

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/ertugrulhaskan/engram/internal/memory"
)

// TestStoreLastChange promotes a memory into a scratch store and asserts the
// reported time matches the promote commit — and that failures (unknown id,
// empty id) come back as errors, never as fabricated times.
func TestStoreLastChange(t *testing.T) {
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

	localMem := filepath.Join(root, "myproj", "memory")
	if err := os.MkdirAll(localMem, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(localMem, "note.md")
	raw := "---\nname: note\n---\n# note\n\nbody\n"
	if err := os.WriteFile(p, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	before := time.Now().Add(-time.Minute)
	if _, err := Promote(p, "github.com/acme/app"); err != nil {
		t.Fatalf("Promote: %v", err)
	}

	stamped, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	meta, ok, err := memory.ReadEngram(string(stamped))
	if err != nil || !ok || meta.ID == "" {
		t.Fatalf("promoted file has no engram id (ok=%v err=%v)", ok, err)
	}

	got, err := StoreLastChange(meta.ID)
	if err != nil {
		t.Fatalf("StoreLastChange: %v", err)
	}
	if got.Before(before) || got.After(time.Now().Add(time.Minute)) {
		t.Errorf("store time %v not within the promote window", got)
	}

	if _, err := StoreLastChange("no-such-id"); err == nil {
		t.Error("unknown id: want error, got a time")
	}
	if _, err := StoreLastChange(""); err == nil {
		t.Error("empty id: want error, got a time")
	}
}
