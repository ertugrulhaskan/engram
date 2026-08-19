package team

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ertugrulhaskan/engram/internal/memory"
)

// useInstall points the process at one engram "install": its own managed team
// store — Dir() derives from XDG_CONFIG_HOME through config.Dir — and its own git
// identity, which is what the owner guard reads back out of the store. Calling it
// again switches sides.
//
// No new mechanism is needed for this: hermeticGitEnv already injects
// XDG_CONFIG_HOME, and this only varies it per side. The one real constraint is
// that t.Setenv is process-wide, so the two installs are exercised sequentially
// rather than concurrently, and a test using this must not call t.Parallel. That
// costs nothing here — every scenario below is inherently ordered (A promotes,
// then B pulls).
func useInstall(t *testing.T, root, email string) {
	t.Helper()
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(root, "gitconfig")
	if err := os.WriteFile(cfg, []byte("[user]\n\tname = "+email+"\n\temail = "+email+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "xdg"))
	t.Setenv("GIT_CONFIG_GLOBAL", cfg)
}

// soleMemory returns the one memory file in dir, ignoring the MEMORY.md index
// that Pull reconciles. It fails rather than guessing when the count is not one,
// so a test asserting "the memory arrived" cannot pass on the wrong file.
func soleMemory(t *testing.T, dir string) string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var found []string
	for _, e := range entries {
		if e.IsDir() || e.Name() == "MEMORY.md" || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		found = append(found, filepath.Join(dir, e.Name()))
	}
	if len(found) != 1 {
		t.Fatalf("want exactly one memory in %s, found %v", dir, found)
	}
	return found[0]
}

// TestSymmetricPromoteAndPullBetweenTwoInstalls closes the gap the other
// two-clone tests leave open. They do build a bare remote and a second clone, but
// that clone is driven by raw git and hand-written frontmatter — so only one side
// ever runs engram code. A hand-written teammate file encodes the test author's
// belief about what a promoted memory looks like; if Promote changed what it
// writes (a renamed key, a different anchor input, reordered frontmatter) those
// fixtures would keep asserting the old shape and stay green while two real
// installs disagreed.
//
// Here both sides call real entry points against one bare remote: A promotes, B
// pulls, and what B reads is whatever A actually wrote. Nothing about the stored
// format is restated by the test, so a drift between what engram writes and what
// engram expects to read fails this immediately.
func TestSymmetricPromoteAndPullBetweenTwoInstalls(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	root := t.TempDir()
	hermeticGitEnv(t, root) // baseline isolation; useInstall varies the per-install bits

	bare := filepath.Join(root, "remote.git")
	gitT(t, "", "init", "--bare", bare)

	const (
		key    = "github.com/acme/app"
		emailA = "ada@example.com"
		emailB = "grace@example.com"
	)
	rootA := filepath.Join(root, "installA")
	rootB := filepath.Join(root, "installB")

	// Each install has its own checkout of the same project, so the two memory
	// dirs are distinct on disk exactly as they would be on two machines.
	memDirA := filepath.Join(root, "checkoutA", "memory")
	memDirB := filepath.Join(root, "checkoutB", "memory")
	for _, d := range []string{memDirA, memDirB} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	targetsB := []ProjectTarget{{Key: key, MemoryDir: memDirB}}

	// --- A promotes, through engram rather than git ---
	useInstall(t, rootA, emailA)
	if err := InitTeam("file://" + bare); err != nil {
		t.Fatalf("A InitTeam: %v", err)
	}
	pathA := filepath.Join(memDirA, "deploy.md")
	const marker = "Staging first, always."
	body := "---\nname: deploy\ndescription: how we ship\nmetadata:\n  type: project\n---\n\n" + marker + "\n"
	if err := os.WriteFile(pathA, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	pushed, err := Promote(pathA, key)
	if err != nil {
		t.Fatalf("A Promote: %v", err)
	}
	if !pushed {
		t.Fatal("A Promote did not reach the remote — nothing after this would prove anything")
	}

	// --- B pulls, through engram rather than git ---
	useInstall(t, rootB, emailB)
	if err := InitTeam("file://" + bare); err != nil {
		t.Fatalf("B InitTeam: %v", err)
	}
	res, err := Pull(targetsB)
	if err != nil {
		t.Fatalf("B Pull: %v", err)
	}
	if res.Placed != 1 {
		t.Fatalf("B Pull = %+v, want Placed=1", res)
	}

	pathB := soleMemory(t, memDirB)
	rawB, err := os.ReadFile(pathB)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(rawB), marker) {
		t.Errorf("B's copy lost the body A promoted:\n%s", rawB)
	}

	// The frontmatter B reads is the frontmatter A wrote — never a fixture. Owner
	// is the sharpest check: it can only be A's email if the promoter's identity
	// survived the round trip through the store.
	metaB, ok, err := memory.ReadEngram(string(rawB))
	if err != nil || !ok {
		t.Fatalf("B's copy has no readable engram block (ok=%v): %v\n%s", ok, err, rawB)
	}
	if metaB.Owner != emailA {
		t.Errorf("owner = %q, want %q — the promoter's identity did not survive the round trip", metaB.Owner, emailA)
	}
	if metaB.Scope != "team" {
		t.Errorf("scope = %q, want team", metaB.Scope)
	}
	if metaB.ID == "" {
		t.Error("id is empty — Pull matches by id, so an unstamped copy would never sync again")
	}
	if metaB.SyncedHash == "" {
		t.Error("syncedHash is empty — without an anchor B can never be told apart from a local edit")
	}

	// And engram's own reading of that state agrees: B is synced, not merely
	// present. This is the assertion the one-sided tests cannot make.
	states, err := SyncStates([]memory.Memory{{Path: pathB, Raw: string(rawB)}})
	if err != nil {
		t.Fatalf("B SyncStates: %v", err)
	}
	if states[pathB] != StateSynced {
		t.Errorf("B state = %v, want StateSynced", states[pathB])
	}

	// A second pull is a no-op rather than a re-place, which is what proves B's
	// anchor matches the store instead of merely looking similar.
	if res, err = Pull(targetsB); err != nil {
		t.Fatalf("B Pull (second): %v", err)
	}
	if res.UpToDate != 1 || res.Placed != 0 || res.Conflicts != 0 {
		t.Errorf("second B Pull = %+v, want UpToDate=1 with nothing placed or conflicting", res)
	}

	// --- A withdraws; the tombstone must reach B through engram on both ends ---
	useInstall(t, rootA, emailA)
	if _, err := Withdraw(pathA); err != nil {
		t.Fatalf("A Withdraw: %v", err)
	}

	useInstall(t, rootB, emailB)
	res, err = Pull(targetsB)
	if err != nil {
		t.Fatalf("B Pull (after withdraw): %v", err)
	}
	// B is not the owner, so B's copy is removed rather than demoted — the
	// owner-side demote path is covered by TestPullDemotesOwnWithdrawnCopy.
	if res.Removed != 1 || res.Demoted != 0 {
		t.Errorf("B Pull after withdraw = %+v, want Removed=1 Demoted=0", res)
	}
	if _, err := os.Stat(pathB); !os.IsNotExist(err) {
		t.Errorf("B's withdrawn copy still exists at %s (stat err = %v)", pathB, err)
	}
}
