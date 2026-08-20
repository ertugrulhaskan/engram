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

// theirsSection extracts the team side of a resolve temp file — the text between
// the "=======" divider and the ">>>>>>> team" closer — which is what a user keeps
// when they resolve by starting from the store's version. Extracting it from the
// real merge text (rather than re-reading the store) keeps the test on the same
// path a user's $EDITOR session takes.
func theirsSection(t *testing.T, merged string) string {
	t.Helper()
	mid := strings.Index(merged, "\n"+conflictMid+"\n")
	end := strings.Index(merged, conflictEnd)
	if mid < 0 || end < 0 || end <= mid {
		t.Fatalf("merge text is missing its markers:\n%s", merged)
	}
	return merged[mid+1+len(conflictMid)+1 : end]
}

// TestSymmetricConflictAndResolveBetweenTwoInstalls covers the one sync state
// whose resolution *writes*: conflict. Every other state either takes the store
// copy verbatim or leaves the local file alone, so conflict is the one path where
// two installs can end up disagreeing about what "resolved" means — and the
// existing conflict tests (resolve_test.go) only ever run one side, advancing the
// store by hand-restamping a fixture rather than through a second install's
// Promote.
//
// Here the divergence is real on both ends: A edits and re-promotes (so the store
// advance is whatever Promote actually writes), B edits locally, and B's pull must
// flag the conflict without touching B's file. B then resolves by KEEPING A MERGE
// of both edits — deliberately not "take theirs", because after take-theirs the
// content matches the store and relationOf short-circuits on that match, so a
// broken re-anchor is invisible. A kept merge is the only resolution whose state
// (local-ahead, promotable) depends on FinishConflictResolve re-basing the anchor
// on the store version; asserting it pins the half of Finish's contract no other
// test reads. B then promotes the merge and A pulls it as a clean fast-forward —
// a resolve on B must never strand A — until both installs hold the same bytes.
func TestSymmetricConflictAndResolveBetweenTwoInstalls(t *testing.T) {
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
		lineA  = "Canary before a wide rollout.\n"
		lineB  = "Never ship on Fridays.\n"
	)
	baseBody := "# Deploy\n\nShip through staging first.\n"
	rootA := filepath.Join(root, "installA")
	rootB := filepath.Join(root, "installB")
	memDirA := filepath.Join(root, "checkoutA", "memory")
	memDirB := filepath.Join(root, "checkoutB", "memory")
	for _, d := range []string{memDirA, memDirB} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	targetsA := []ProjectTarget{{Key: key, MemoryDir: memDirA}}
	targetsB := []ProjectTarget{{Key: key, MemoryDir: memDirB}}

	// --- shared base: A promotes, B pulls (the opening of the promote/pull test) ---
	useInstall(t, rootA, emailA)
	if err := InitTeam("file://" + bare); err != nil {
		t.Fatalf("A InitTeam: %v", err)
	}
	pathA := filepath.Join(memDirA, "deploy.md")
	body := "---\nname: deploy\ndescription: how we ship\nmetadata:\n  type: project\n---\n\n" + baseBody
	if err := os.WriteFile(pathA, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if pushed, err := Promote(pathA, key); err != nil || !pushed {
		t.Fatalf("A Promote: pushed=%v err=%v", pushed, err)
	}

	useInstall(t, rootB, emailB)
	if err := InitTeam("file://" + bare); err != nil {
		t.Fatalf("B InitTeam: %v", err)
	}
	if res, err := Pull(targetsB); err != nil || res.Placed != 1 {
		t.Fatalf("B Pull = %+v err=%v, want Placed=1", res, err)
	}
	pathB := soleMemory(t, memDirB)

	// --- both sides edit past the shared anchor; A promotes again, so the store moves ---
	useInstall(t, rootA, emailA)
	rawA, err := os.ReadFile(pathA)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pathA, []byte(setBody(string(rawA), baseBody+lineA)), 0o644); err != nil {
		t.Fatal(err)
	}
	if pushed, err := Promote(pathA, key); err != nil || !pushed {
		t.Fatalf("A re-Promote: pushed=%v err=%v", pushed, err)
	}

	useInstall(t, rootB, emailB)
	rawB, err := os.ReadFile(pathB)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pathB, []byte(setBody(string(rawB), baseBody+lineB)), 0o644); err != nil {
		t.Fatal(err)
	}

	// --- B's pull flags the conflict and leaves B's file exactly as B wrote it ---
	before, err := os.ReadFile(pathB)
	if err != nil {
		t.Fatal(err)
	}
	res, err := Pull(targetsB)
	if err != nil {
		t.Fatalf("B Pull (diverged): %v", err)
	}
	if res.Conflicts != 1 || res.Updated != 0 || res.Placed != 0 {
		t.Fatalf("B Pull (diverged) = %+v, want Conflicts=1 and nothing written", res)
	}
	after, err := os.ReadFile(pathB)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatalf("a conflicting pull modified B's file:\n--- before ---\n%s--- after ---\n%s", before, after)
	}
	if st, err := SyncStates([]memory.Memory{{Path: pathB, Raw: string(after)}}); err != nil || st[pathB] != StateDiverged {
		t.Fatalf("B state = %v (err=%v), want StateDiverged", st[pathB], err)
	}

	// --- B resolves, keeping a merge of both edits ---
	tmp, err := BeginConflictResolve(pathB)
	if err != nil {
		t.Fatalf("B BeginConflictResolve: %v", err)
	}
	mergedRaw, err := os.ReadFile(tmp)
	if err != nil {
		t.Fatal(err)
	}
	// Both real sides must be in the merge text: B's local edit and whatever A's
	// re-promote actually stored — never a fixture's idea of it.
	if !strings.Contains(string(mergedRaw), strings.TrimSuffix(lineB, "\n")) ||
		!strings.Contains(string(mergedRaw), strings.TrimSuffix(lineA, "\n")) {
		t.Fatalf("merge text is missing a side:\n%s", mergedRaw)
	}
	// The user's editor action: start from the team side, re-add the local line.
	resolution := strings.Replace(theirsSection(t, string(mergedRaw)), lineA, lineA+lineB, 1)
	if err := os.WriteFile(tmp, []byte(resolution), 0o644); err != nil {
		t.Fatal(err)
	}
	if resolved, err := FinishConflictResolve(pathB, tmp); err != nil || !resolved {
		t.Fatalf("B FinishConflictResolve: resolved=%v err=%v", resolved, err)
	}

	// A kept merge reads local-ahead — resolved and promotable, never re-conflicting.
	// This is the assertion that needs the anchor re-based on the store version:
	// against the stale anchor the same content would read StateDiverged.
	rawB2, err := os.ReadFile(pathB)
	if err != nil {
		t.Fatal(err)
	}
	if st, err := SyncStates([]memory.Memory{{Path: pathB, Raw: string(rawB2)}}); err != nil || st[pathB] != StateLocalAhead {
		t.Fatalf("B state after merge-resolve = %v (err=%v), want StateLocalAhead", st[pathB], err)
	}
	if res, err = Pull(targetsB); err != nil || res.Ahead != 1 || res.Conflicts != 0 {
		t.Fatalf("B Pull (post-resolve) = %+v err=%v, want Ahead=1 with no re-conflict", res, err)
	}

	// --- B shares the resolution; B now reads synced and pulls clean ---
	if pushed, err := Promote(pathB, key); err != nil || !pushed {
		t.Fatalf("B Promote (merge): pushed=%v err=%v", pushed, err)
	}
	if res, err = Pull(targetsB); err != nil || res.UpToDate != 1 || res.Conflicts != 0 {
		t.Fatalf("B Pull (after sharing) = %+v err=%v, want UpToDate=1", res, err)
	}

	// --- A takes B's resolution as a clean fast-forward — the resolve did not strand A ---
	useInstall(t, rootA, emailA)
	res, err = Pull(targetsA)
	if err != nil {
		t.Fatalf("A Pull (resolution): %v", err)
	}
	if res.Updated != 1 || res.Conflicts != 0 {
		t.Fatalf("A Pull (resolution) = %+v, want Updated=1 Conflicts=0", res)
	}
	rawA2, err := os.ReadFile(pathA)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(rawA2), strings.TrimSuffix(lineA, "\n")) ||
		!strings.Contains(string(rawA2), strings.TrimSuffix(lineB, "\n")) {
		t.Errorf("A's copy lost an edit in the round trip:\n%s", rawA2)
	}
	if st, err := SyncStates([]memory.Memory{{Path: pathA, Raw: string(rawA2)}}); err != nil || st[pathA] != StateSynced {
		t.Errorf("A state after taking the resolution = %v (err=%v), want StateSynced", st[pathA], err)
	}
	// The two installs end with the same content — compared exactly as Pull
	// compares it — so they agree about what "resolved" means.
	rawB3, err := os.ReadFile(pathB)
	if err != nil {
		t.Fatal(err)
	}
	if !sameContent(string(rawA2), string(rawB3)) {
		t.Errorf("the installs disagree after the round trip:\n--- A ---\n%s--- B ---\n%s", rawA2, rawB3)
	}
}
