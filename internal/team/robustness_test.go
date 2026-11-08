package team

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ertugrulhaskan/engram/internal/memory"
)

// These tests pin what happens when the store clone and its remote disagree:
// a teammate pushed first, the remote was unreachable, or one memory id ended up
// under two store paths. Every case here was a real defect — a store that wedged
// after one rejected push, a withdraw that could not be retried and left the
// memory readable by the team, a withdraw that removed only one of two copies,
// and a promote that reverted a teammate's newer version.

// mateClone clones the bare remote as a teammate would, for setting up pushes
// that race the engram store.
func mateClone(t *testing.T, root, bare string) string {
	t.Helper()
	mate := filepath.Join(root, "mate")
	gitT(t, "", "clone", "file://"+bare, mate)
	return mate
}

// matePush writes rel under the teammate's clone, commits it and pushes.
func matePush(t *testing.T, mate, rel, content string) {
	t.Helper()
	gitT(t, mate, "pull", "--ff-only")
	p := filepath.Join(mate, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	gitT(t, mate, "add", "-A")
	gitT(t, mate, "commit", "-m", "teammate: "+rel)
	gitT(t, mate, "push")
}

// remoteHas reports whether a fresh clone of the bare remote contains rel —
// the only honest answer to "what can the team read".
func remoteHas(t *testing.T, root, bare, rel string) bool {
	t.Helper()
	dst, err := os.MkdirTemp(root, "check-")
	if err != nil {
		t.Fatal(err)
	}
	gitT(t, "", "clone", "-q", "file://"+bare, filepath.Join(dst, "c"))
	_, err = os.Stat(filepath.Join(dst, "c", rel))
	return err == nil
}

// offline makes the remote unreachable until the returned func restores it.
func offline(t *testing.T, bare string) (restore func()) {
	t.Helper()
	away := bare + ".away"
	if err := os.Rename(bare, away); err != nil {
		t.Fatal(err)
	}
	return func() {
		if err := os.Rename(away, bare); err != nil {
			t.Fatal(err)
		}
	}
}

func scopeOf(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	m, _, err := memory.ReadEngram(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	return m.Scope
}

// A1: a teammate pushed first. SPEC §7 promises a rebase and one retry; before
// the fix the push was rejected, pushed=false blamed creds, and every later pull
// failed because --ff-only cannot move a diverged store.
func TestPromoteRebasesOverTeammatePush(t *testing.T) {
	root, bare, writeMem := batchStore(t)
	mate := mateClone(t, root, bare)
	matePush(t, mate, "projects/github.com/acme/app/theirs.md",
		"---\nname: theirs\nengram:\n    id: ID-THEIRS\n    scope: team\n---\nfrom a teammate\n")

	mine := writeMem("mine.md", "mine")
	pushed, err := Promote(mine, "github.com/acme/app")
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if !pushed {
		t.Fatal("push was rejected and not retried after a rebase")
	}
	for _, rel := range []string{"projects/github.com/acme/app/mine.md", "projects/github.com/acme/app/theirs.md"} {
		if !remoteHas(t, root, bare, rel) {
			t.Errorf("remote is missing %s after the rebased push", rel)
		}
	}
	if _, err := PullPlan(nil); err != nil {
		t.Errorf("pull after the promote: %v", err)
	}
}

// A1: a commit left behind by an offline push must not wedge the store once the
// remote moves on — pull rebases it and flushes it.
func TestPullFlushesCommitLeftByOfflinePush(t *testing.T) {
	root, bare, writeMem := batchStore(t)
	mate := mateClone(t, root, bare)

	restore := offline(t, bare)
	mine := writeMem("mine.md", "mine")
	pushed, err := Promote(mine, "github.com/acme/app")
	restore()
	if err != nil {
		t.Fatalf("Promote while offline: %v", err)
	}
	if pushed {
		t.Fatal("precondition: an offline promote should not report pushed")
	}

	matePush(t, mate, "global/theirs.md", "---\nname: theirs\nengram:\n    id: ID-T\n    scope: team\n---\nx\n")
	if _, err := PullPlan(nil); err != nil {
		t.Fatalf("pull wedged on the unpushed local commit: %v", err)
	}
	// The plan is a preview: it rebases locally but must not reach the remote.
	if remoteHas(t, root, bare, "projects/github.com/acme/app/mine.md") {
		t.Error("the pull PLAN pushed to the remote; only a confirmed pull may")
	}
	if _, err := PullApply(nil); err != nil {
		t.Fatalf("PullApply: %v", err)
	}
	if !remoteHas(t, root, bare, "projects/github.com/acme/app/mine.md") {
		t.Error("the confirmed pull did not flush the commit the offline promote left behind")
	}
}

// A dirty store clone (a promote that failed partway through staging) must not
// block a pull; the rebase carries the leftover across with --autostash.
func TestPullToleratesADirtyStoreClone(t *testing.T) {
	root, bare, _ := batchStore(t)
	mate := mateClone(t, root, bare)
	store, _ := Dir()
	if err := os.WriteFile(filepath.Join(store, "global", "leftover.md"), []byte("half-promoted\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitT(t, store, "add", "--", "global/leftover.md")
	matePush(t, mate, "global/theirs.md", "x\n")
	if _, err := PullPlan(nil); err != nil {
		t.Fatalf("a dirty clone blocked the pull: %v", err)
	}
	if _, err := os.Stat(filepath.Join(store, "global", "theirs.md")); err != nil {
		t.Error("the pull did not bring the teammate's file in")
	}
}

// A re-promote (which REMOVES a tombstone) racing a teammate's withdraw (which
// appends one) must keep the removal. Git's union merge kept both lines, so the
// re-shared memory's tombstone came back and the next pull deleted it from
// every teammate's machine.
func TestRepromoteRacingAWithdrawKeepsItsTombstoneRemoved(t *testing.T) {
	root, bare, writeMem := batchStore(t)
	mine := writeMem("mine.md", "body")
	if _, err := Promote(mine, "github.com/acme/app"); err != nil {
		t.Fatal(err)
	}
	if pushed, err := Withdraw(mine); err != nil || !pushed {
		t.Fatalf("Withdraw = (%v, %v)", pushed, err)
	}
	meta, _, _ := memory.ReadEngram(string(must(os.ReadFile(mine))))
	mate := mateClone(t, root, bare)
	gitT(t, mate, "pull", "--ff-only")
	ledger := string(must(os.ReadFile(filepath.Join(mate, withdrawnLedger))))
	matePush(t, mate, withdrawnLedger, ledger+"ID-OTHER\tother.md\n")

	if pushed, err := Promote(mine, "github.com/acme/app"); err != nil || !pushed {
		t.Fatalf("re-Promote = (%v, %v), want pushed", pushed, err)
	}
	dst := filepath.Join(root, "verify")
	gitT(t, "", "clone", "-q", "file://"+bare, dst)
	got := ledgerIDs(string(must(os.ReadFile(filepath.Join(dst, withdrawnLedger)))))
	if got[meta.ID] {
		t.Error("the re-promoted memory's tombstone came back; the next pull would delete it from teammates")
	}
	if !got["ID-OTHER"] {
		t.Error("the teammate's concurrent withdrawal was lost")
	}
	if !remoteHas(t, root, bare, "projects/github.com/acme/app/mine.md") {
		t.Error("the re-promoted memory is not on the remote")
	}
}

// mergeLedger keeps what either side added and drops what either side removed.
func TestMergeLedgerByID(t *testing.T) {
	base := "X\tx.md\nK\tk.md\n"
	a := "K\tk.md\nA\ta.md\n"          // removed X, added A
	b := "X\tx.md\nK\tk.md\nB\tb.md\n" // kept X, added B
	got := ledgerIDs(mergeLedger(base, a, b))
	for id, want := range map[string]bool{"X": false, "K": true, "A": true, "B": true} {
		if got[id] != want {
			t.Errorf("merged ledger has %s=%v, want %v (got %v)", id, got[id], want, got)
		}
	}
}

// A symlinked ledger means no tombstone can be written, so withdraw must refuse
// up front instead of removing the store copy and reporting success.
func TestWithdrawRefusesWithASymlinkedLedger(t *testing.T) {
	root, bare, writeMem := batchStore(t)
	mine := writeMem("mine.md", "body")
	if _, err := Promote(mine, "github.com/acme/app"); err != nil {
		t.Fatal(err)
	}
	store, _ := Dir()
	_ = os.Remove(filepath.Join(store, withdrawnLedger))
	if err := os.Symlink(filepath.Join(root, "elsewhere"), filepath.Join(store, withdrawnLedger)); err != nil {
		t.Fatal(err)
	}
	if _, err := Withdraw(mine); err == nil {
		t.Fatal("Withdraw succeeded with a symlinked ledger; want a refusal")
	}
	if !remoteHas(t, root, bare, "projects/github.com/acme/app/mine.md") || scopeOf(t, mine) != "team" {
		t.Error("a refused withdraw still changed the store or the local memory")
	}
}

// A1: when the rebase itself conflicts, engram aborts it, leaves the store
// clone clean, and says the store diverged — not that the credentials failed.
func TestPushConflictReportsDivergedAndLeavesStoreClean(t *testing.T) {
	root, bare, _ := batchStore(t)
	mate := mateClone(t, root, bare)
	store, _ := Dir()

	matePush(t, mate, "global/same.md", "teammate version\n")
	if err := os.MkdirAll(filepath.Join(store, "global"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store, "global", "same.md"), []byte("my version\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitT(t, store, "add", "-A")
	gitT(t, store, "commit", "-m", "mine")

	pushed, err := pushStore(store)
	if pushed || !errors.Is(err, ErrStoreDiverged) {
		t.Fatalf("pushStore = (%v, %v), want (false, ErrStoreDiverged)", pushed, err)
	}
	if _, statErr := os.Stat(filepath.Join(store, ".git", "rebase-merge")); statErr == nil {
		t.Error("a rebase was left in progress in the store clone")
	}
	if out, _ := runGitCapture(store, "status", "--porcelain"); out != "" {
		t.Errorf("store clone left dirty:\n%s", out)
	}
}

// A1: two people withdrawing at once both append to the tombstone ledger; the
// rebase must merge those appends rather than conflict.
func TestConcurrentWithdrawLedgerAppendsMerge(t *testing.T) {
	root, bare, writeMem := batchStore(t)
	mine := writeMem("mine.md", "mine")
	if _, err := Promote(mine, "github.com/acme/app"); err != nil {
		t.Fatal(err)
	}
	mate := mateClone(t, root, bare)
	gitT(t, mate, "pull", "--ff-only")
	ledger, _ := os.ReadFile(filepath.Join(mate, withdrawnLedger))
	matePush(t, mate, withdrawnLedger, string(ledger)+"ID-OTHER\tother.md\n")

	pushed, err := Withdraw(mine)
	if err != nil || !pushed {
		t.Fatalf("Withdraw = (%v, %v), want pushed", pushed, err)
	}
	store, _ := Dir()
	got := readWithdrawn(store)
	meta, _, _ := memory.ReadEngram(string(must(os.ReadFile(mine))))
	if !got["ID-OTHER"] || !got[meta.ID] {
		t.Errorf("ledger after the merge = %v, want both the teammate's and this withdrawal", got)
	}
}

// A2: a withdraw whose push fails must leave the memory shared, so it can be
// retried. Before the fix it flipped to personal anyway, the guard then refused
// a retry ("isn't shared"), and the team could read the memory indefinitely.
func TestWithdrawOfflineCanBeRetried(t *testing.T) {
	root, bare, writeMem := batchStore(t)
	mine := writeMem("mine.md", "secret-ish")
	if _, err := Promote(mine, "github.com/acme/app"); err != nil {
		t.Fatal(err)
	}
	rel := "projects/github.com/acme/app/mine.md"

	restore := offline(t, bare)
	pushed, err := Withdraw(mine)
	restore()
	if err != nil {
		t.Fatalf("offline Withdraw: %v", err)
	}
	if pushed {
		t.Fatal("precondition: an offline withdraw should not report pushed")
	}
	if got := scopeOf(t, mine); got != "team" {
		t.Fatalf("after a failed push the memory reads scope=%q; it must stay team so a retry works", got)
	}

	pushed, err = Withdraw(mine)
	if err != nil || !pushed {
		t.Fatalf("retry Withdraw = (%v, %v), want pushed", pushed, err)
	}
	if remoteHas(t, root, bare, rel) {
		t.Error("the team can still read the memory after the retried withdraw")
	}
	if got := scopeOf(t, mine); got != "personal" {
		t.Errorf("after a pushed withdraw scope=%q, want personal", got)
	}
}

// A3: one id under two store paths (a rename then re-promote, or a re-promote
// to another placement — both supported, see storeIndexByID). Withdraw must
// remove every copy; before the fix it removed one and reported success.
func TestWithdrawRemovesEveryCopy(t *testing.T) {
	cases := []struct {
		name   string
		second func(t *testing.T, mine string) (string, string) // returns (path to withdraw, second store rel)
	}{
		{"renamed then re-promoted", func(t *testing.T, mine string) (string, string) {
			renamed := filepath.Join(filepath.Dir(mine), "mine-renamed.md")
			if err := os.Rename(mine, renamed); err != nil {
				t.Fatal(err)
			}
			if _, err := Promote(renamed, "github.com/acme/app"); err != nil {
				t.Fatal(err)
			}
			return renamed, "projects/github.com/acme/app/mine-renamed.md"
		}},
		{"re-promoted to global", func(t *testing.T, mine string) (string, string) {
			if _, err := Promote(mine, "global"); err != nil {
				t.Fatal(err)
			}
			return mine, "global/mine.md"
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root, bare, writeMem := batchStore(t)
			mine := writeMem("mine.md", "body")
			if _, err := Promote(mine, "github.com/acme/app"); err != nil {
				t.Fatal(err)
			}
			first := "projects/github.com/acme/app/mine.md"
			path, second := tc.second(t, mine)
			if !remoteHas(t, root, bare, first) || !remoteHas(t, root, bare, second) {
				t.Fatal("precondition: both copies should be on the remote")
			}
			if pushed, err := Withdraw(path); err != nil || !pushed {
				t.Fatalf("Withdraw = (%v, %v)", pushed, err)
			}
			for _, rel := range []string{first, second} {
				if remoteHas(t, root, bare, rel) {
					t.Errorf("withdraw left %s readable by the team", rel)
				}
			}
		})
	}
}

// A4: promote must not revert a newer teammate version. Before the fix the
// same-id store copy was overwritten and the anchor re-stamped, hiding it.
func TestPromoteRefusesWhenTheTeamCopyMoved(t *testing.T) {
	for _, editLocal := range []bool{false, true} { // incoming, then diverged
		name := map[bool]string{false: "local unchanged (incoming)", true: "local edited too (diverged)"}[editLocal]
		t.Run(name, func(t *testing.T) {
			root, bare, writeMem := batchStore(t)
			mine := writeMem("mine.md", "v1")
			if _, err := Promote(mine, "github.com/acme/app"); err != nil {
				t.Fatal(err)
			}
			rel := "projects/github.com/acme/app/mine.md"
			mate := mateClone(t, root, bare)
			gitT(t, mate, "pull", "--ff-only")
			theirs := strings.Replace(string(must(os.ReadFile(filepath.Join(mate, rel)))), "v1", "v2 from a teammate", 1)
			matePush(t, mate, rel, theirs)
			if _, err := PullPlan(nil); err != nil { // the store clone now holds v2
				t.Fatal(err)
			}
			if editLocal {
				raw := must(os.ReadFile(mine))
				if err := os.WriteFile(mine, []byte(strings.Replace(string(raw), "v1", "v1 edited here", 1)), 0o644); err != nil {
					t.Fatal(err)
				}
			}

			if _, err := Promote(mine, "github.com/acme/app"); err == nil {
				t.Fatal("promote overwrote a newer team copy; want a refusal")
			}
			dst := filepath.Join(root, "verify")
			gitT(t, "", "clone", "-q", "file://"+bare, dst)
			if got := string(must(os.ReadFile(filepath.Join(dst, rel)))); !strings.Contains(got, "v2 from a teammate") {
				t.Errorf("the teammate's version was lost from the remote:\n%s", got)
			}
		})
	}
}

func must(b []byte, err error) []byte {
	if err != nil {
		panic(err)
	}
	return b
}

// Hardening: the tombstone ledger is never followed through a symlink, like
// every other store read and write — appending to one would write outside the
// store, wherever a teammate's committed link pointed.
func TestLedgerIsNotFollowedThroughASymlink(t *testing.T) {
	store, outside := t.TempDir(), t.TempDir()
	victim := filepath.Join(outside, "victim")
	if err := os.WriteFile(victim, []byte("untouched\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(victim, filepath.Join(store, withdrawnLedger)); err != nil {
		t.Fatal(err)
	}
	if rel := addWithdrawn(store, "ID-1", "x.md"); rel != "" {
		t.Errorf("addWithdrawn wrote through the symlinked ledger (returned %q)", rel)
	}
	if got := string(must(os.ReadFile(victim))); got != "untouched\n" {
		t.Errorf("the file outside the store was modified:\n%s", got)
	}
}

// The rebase-merge path itself, built directly: the clone has a local commit
// removing tombstone X while a teammate appended Y. pushStore must merge the
// ledger by id — X stays removed, Y arrives — and push. (Promote and withdraw
// now sync first, so this race only happens when a teammate pushes between the
// sync and the push; this pins it regardless.)
func TestPushResolvesALedgerOnlyConflictByID(t *testing.T) {
	root, bare, _ := batchStore(t)
	store, _ := Dir()
	ledgerPath := filepath.Join(store, withdrawnLedger)
	if err := os.WriteFile(ledgerPath, []byte("ID-X\tx.md\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitT(t, store, "add", "-A")
	gitT(t, store, "commit", "-m", "tombstone X")
	gitT(t, store, "push")

	mate := mateClone(t, root, bare)
	matePush(t, mate, withdrawnLedger, "ID-X\tx.md\nID-Y\ty.md\n")

	if err := os.WriteFile(ledgerPath, []byte(""), 0o644); err != nil { // re-promote removes X
		t.Fatal(err)
	}
	gitT(t, store, "add", "-A")
	gitT(t, store, "commit", "-m", "re-promote X")
	pushed, err := pushStore(store)
	if err != nil || !pushed {
		t.Fatalf("pushStore = (%v, %v), want pushed after merging the ledger", pushed, err)
	}
	dst := filepath.Join(root, "verify")
	gitT(t, "", "clone", "-q", "file://"+bare, dst)
	got := ledgerIDs(string(must(os.ReadFile(filepath.Join(dst, withdrawnLedger)))))
	if got["ID-X"] || !got["ID-Y"] {
		t.Errorf("remote ledger = %v, want Y without X", got)
	}
}

// Promote syncs before judging, so a teammate's newer version is caught even
// when the user hasn't pulled it yet.
func TestPromoteRefusesAnUnpulledNewerTeamVersion(t *testing.T) {
	root, bare, writeMem := batchStore(t)
	mine := writeMem("mine.md", "v1")
	if _, err := Promote(mine, "github.com/acme/app"); err != nil {
		t.Fatal(err)
	}
	rel := "projects/github.com/acme/app/mine.md"
	mate := mateClone(t, root, bare)
	gitT(t, mate, "pull", "--ff-only")
	theirs := strings.Replace(string(must(os.ReadFile(filepath.Join(mate, rel)))), "v1", "v2 from a teammate", 1)
	matePush(t, mate, rel, theirs)

	if _, err := Promote(mine, "github.com/acme/app"); err == nil {
		t.Fatal("promote overwrote an unpulled newer team copy; want a refusal")
	}
	dst := filepath.Join(root, "verify")
	gitT(t, "", "clone", "-q", "file://"+bare, dst)
	if got := string(must(os.ReadFile(filepath.Join(dst, rel)))); !strings.Contains(got, "v2 from a teammate") {
		t.Errorf("the teammate's version was lost:\n%s", got)
	}
}

// A push the server refuses (hook, branch protection) is not "offline": it
// must come back as an error carrying the server's reason, not pushed=false.
func TestPushRefusedByTheServerSaysWhy(t *testing.T) {
	_, bare, writeMem := batchStore(t)
	hook := filepath.Join(bare, "hooks", "pre-receive")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\necho 'branch is protected' >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	mine := writeMem("mine.md", "body")
	pushed, err := Promote(mine, "github.com/acme/app")
	if pushed || err == nil {
		t.Fatalf("Promote = (%v, %v), want a refusal error", pushed, err)
	}
	if !strings.Contains(err.Error(), "didn't accept") || !strings.Contains(err.Error(), "branch is protected") {
		t.Errorf("the error doesn't carry the server's reason: %v", err)
	}
}

// A refused promote must leave nothing claiming to be shared: the local file is
// restored and the clone keeps no commit the team never received.
func TestRefusedPromoteRollsBack(t *testing.T) {
	_, bare, writeMem := batchStore(t)
	store, _ := Dir()
	before := headSHA(t, store)
	if err := os.WriteFile(filepath.Join(bare, "hooks", "pre-receive"), []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	mine := writeMem("mine.md", "body")
	orig := string(must(os.ReadFile(mine)))
	if _, err := Promote(mine, "github.com/acme/app"); err == nil {
		t.Fatal("want a refusal")
	}
	if got := string(must(os.ReadFile(mine))); got != orig {
		t.Errorf("the local memory was left stamped:\n%s", got)
	}
	if after := headSHA(t, store); after != before {
		t.Error("the clone kept the refused promote's commit")
	}
}

// A refused withdraw must not leave a committed tombstone behind: a later pull
// would demote the owner's copy while the remote still serves the memory.
func TestRefusedWithdrawRollsBack(t *testing.T) {
	_, bare, writeMem := batchStore(t)
	mine := writeMem("mine.md", "body")
	if _, err := Promote(mine, "github.com/acme/app"); err != nil {
		t.Fatal(err)
	}
	store, _ := Dir()
	before := headSHA(t, store)
	if err := os.WriteFile(filepath.Join(bare, "hooks", "pre-receive"), []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Withdraw(mine); err == nil {
		t.Fatal("want a refusal")
	}
	if after := headSHA(t, store); after != before {
		t.Error("the clone kept the refused withdraw's commit")
	}
	if _, err := PullPlan(nil); err != nil {
		t.Fatal(err)
	}
	if _, err := PullApply([]ProjectTarget{{Key: "github.com/acme/app", MemoryDir: filepath.Dir(mine)}}); err != nil {
		t.Fatal(err)
	}
	if got := scopeOf(t, mine); got != "team" {
		t.Errorf("after a refused withdraw and a pull the memory reads scope=%q; it is still shared, so team", got)
	}
}

// Withdraw is all-or-nothing: offline, it leaves no pending tombstone commit
// for a later pull to apply before the removal reaches the remote.
func TestOfflineWithdrawLeavesNoPendingCommit(t *testing.T) {
	_, bare, writeMem := batchStore(t)
	mine := writeMem("mine.md", "body")
	if _, err := Promote(mine, "github.com/acme/app"); err != nil {
		t.Fatal(err)
	}
	store, _ := Dir()
	before := headSHA(t, store)
	restore := offline(t, bare)
	pushed, err := Withdraw(mine)
	restore()
	if err != nil || pushed {
		t.Fatalf("offline Withdraw = (%v, %v), want (false, nil)", pushed, err)
	}
	if after := headSHA(t, store); after != before {
		t.Error("an offline withdraw left a pending commit in the clone")
	}
	if scopeOf(t, mine) != "team" {
		t.Error("an offline withdraw changed the local scope")
	}
}

// A promote or withdraw that fails BEFORE its commit must leave the clone with
// nothing staged: the next, unrelated commit would otherwise sweep the leftovers
// in and push them (a staged deletion could remove a team memory for everyone).
func TestFailureBeforeCommitLeavesNothingStaged(t *testing.T) {
	for _, op := range []string{"promote", "withdraw"} {
		t.Run(op, func(t *testing.T) {
			_, _, writeMem := batchStore(t)
			mine := writeMem("mine.md", "body")
			if op == "withdraw" {
				if _, err := Promote(mine, "github.com/acme/app"); err != nil {
					t.Fatal(err)
				}
			}
			orig := string(must(os.ReadFile(mine)))
			store, _ := Dir()
			hook := filepath.Join(store, ".git", "hooks", "pre-commit")
			if err := os.WriteFile(hook, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
				t.Fatal(err)
			}
			var err error
			if op == "promote" {
				_, err = Promote(mine, "github.com/acme/app")
			} else {
				_, err = Withdraw(mine)
			}
			if err == nil {
				t.Fatal("want the commit failure reported")
			}
			if out, _ := runGitCapture(store, "status", "--porcelain"); out != "" {
				t.Errorf("the failed %s left the clone dirty:\n%s", op, out)
			}
			if got := string(must(os.ReadFile(mine))); got != orig {
				t.Errorf("the failed %s changed the local memory:\n%s", op, got)
			}
		})
	}
}

// The rollback of a refused promote keeps an edit made during the push, and
// keeps the file's mode when it does restore.
func TestRestoreLocalKeepsEditsAndMode(t *testing.T) {
	dir := t.TempDir()
	untouched := filepath.Join(dir, "a.md")
	edited := filepath.Join(dir, "b.md")
	for _, p := range []string{untouched, edited} {
		if err := os.WriteFile(p, []byte("STAMPED"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(edited, []byte("STAMPED plus an edit made during the push"), 0o600); err != nil {
		t.Fatal(err)
	}
	prepared := []preparedPromotion{
		{memPath: untouched, orig: []byte("ORIGINAL"), stamped: "STAMPED", mode: 0o600},
		{memPath: edited, orig: []byte("ORIGINAL"), stamped: "STAMPED", mode: 0o600},
	}
	err := restoreLocal(prepared, fmt.Errorf("refused"))
	if got := string(must(os.ReadFile(untouched))); got != "ORIGINAL" {
		t.Errorf("unchanged file not restored: %q", got)
	}
	if fi, _ := os.Stat(untouched); fi.Mode().Perm() != 0o600 {
		t.Errorf("restored file mode = %v, want 0600", fi.Mode().Perm())
	}
	if got := string(must(os.ReadFile(edited))); !strings.Contains(got, "an edit made during the push") {
		t.Errorf("an edit made during the push was reverted: %q", got)
	}
	if err == nil || !strings.Contains(err.Error(), "b.md") {
		t.Errorf("the error should name the kept file: %v", err)
	}
}

// mergeLedger with an empty side adds no stray blank line and keeps the other
// side's header.
func TestMergeLedgerWithAnEmptySide(t *testing.T) {
	got := mergeLedger("", "", "# tombstones\nID-Y\ty.md\n")
	if strings.HasPrefix(got, "\n") || !strings.Contains(got, "# tombstones") || !strings.Contains(got, "ID-Y") {
		t.Errorf("merge with an empty side = %q", got)
	}
}

// Network commands run detached; a local commit keeps the terminal (gpg).
func TestGitSubcommand(t *testing.T) {
	cases := map[string][]string{
		"push":   {"-C", "/x", "push"},
		"pull":   {"-C", "/x", "-c", "protocol.ext.allow=never", "pull", "--rebase"},
		"commit": {"-C", "/x", "commit", "-m", "m"},
	}
	for want, args := range cases {
		if got := gitSubcommand(args); got != want {
			t.Errorf("gitSubcommand(%v) = %q, want %q", args, got, want)
		}
	}
}

// A store file whose name holds glob characters (a teammate can commit any name)
// must be staged and restored as that one path. git reads `*` in a pathspec as
// a wildcard even after "--", so a withdraw of `*.md` swept a neighbour's change
// into the commit it pushed, and restorePaths reverted the neighbour.
func TestWithdrawTreatsGlobCharactersLiterally(t *testing.T) {
	root, bare, writeMem := batchStore(t)
	plain := writeMem("plain.md", "plain")
	// writeMem would write `name: *`, a YAML alias marker, so quote it here.
	star := filepath.Join(filepath.Dir(plain), "*.md")
	if err := os.WriteFile(star, []byte("---\nname: \"star\"\nmetadata:\n  type: project\n---\nstar\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Promote(star, "github.com/acme/app"); err != nil {
		t.Fatal(err)
	}
	if _, err := Promote(plain, "github.com/acme/app"); err != nil {
		t.Fatal(err)
	}
	store, _ := Dir()
	neighbour := filepath.Join(store, "projects", "github.com", "acme", "app", "plain.md")
	if _, err := os.Stat(neighbour); err != nil {
		t.Fatalf("store layout changed, fix the test: %v", err)
	}
	if err := os.WriteFile(neighbour, []byte("uncommitted edit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Withdraw(star); err != nil {
		t.Fatal(err)
	}
	if got := string(must(os.ReadFile(neighbour))); got != "uncommitted edit\n" {
		t.Errorf("the neighbour's working copy changed: %q", got)
	}
	dst := filepath.Join(root, "check")
	gitT(t, "", "clone", "-q", "file://"+bare, dst)
	pushed := string(must(os.ReadFile(filepath.Join(dst, "projects", "github.com", "acme", "app", "plain.md"))))
	if strings.Contains(pushed, "uncommitted edit") {
		t.Error("withdrawing *.md pushed the neighbour's uncommitted edit to the team")
	}
}

func TestRestorePathsTreatsGlobCharactersLiterally(t *testing.T) {
	_, _, _ = batchStore(t)
	store, _ := Dir()
	dir := filepath.Join(store, "g")
	mustMkdir(t, dir)
	for name, body := range map[string]string{"*.md": "star\n", "a.md": "a\n"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	gitT(t, store, "add", "-A")
	gitT(t, store, "commit", "-q", "-m", "fixture")
	// A staged edit to the neighbour, then a failed operation that removed *.md.
	if err := os.WriteFile(filepath.Join(dir, "a.md"), []byte("staged edit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitT(t, store, "add", "--", "g/a.md")
	if err := os.Remove(filepath.Join(dir, "*.md")); err != nil {
		t.Fatal(err)
	}
	restorePaths(store, []string{filepath.Join("g", "*.md")})
	if got := string(must(os.ReadFile(filepath.Join(dir, "*.md")))); got != "star\n" {
		t.Errorf("*.md was not restored: %q", got)
	}
	if got := string(must(os.ReadFile(filepath.Join(dir, "a.md")))); got != "staged edit\n" {
		t.Errorf("restoring *.md reverted the neighbour a.md: %q", got)
	}
	if out, _ := runGitCapture(store, "diff", "--cached", "--name-only"); out != "g/a.md" {
		t.Errorf("restoring *.md unstaged the neighbour; staged now: %q", out)
	}
}

// When the rebase in pushStore drops our commit as empty (a teammate pushed the
// same change) and the retried push is refused, HEAD is no longer our commit.
// rollbackCommit's `reset --keep HEAD~1` then rewound whatever sat there: here
// an earlier commit an offline push left for later, so that promote's commit
// vanished while its local file still claimed to be shared.
func TestRollbackAfterARebaseDroppedOurCommit(t *testing.T) {
	root, bare, _ := batchStore(t)
	store, _ := Dir()
	mate := mateClone(t, root, bare)

	writeStore(t, store, "earlier.md", "left by an offline push\n")
	gitT(t, store, "add", "--", "earlier.md")
	gitT(t, store, "commit", "-q", "-m", "earlier, not pushed yet")

	writeStore(t, store, "same.md", "same change\n")
	gitT(t, store, "add", "--", "same.md")
	gitT(t, store, "commit", "-q", "-m", "ours")
	ours, err := commitIdentity(store)
	if err != nil {
		t.Fatal(err)
	}

	matePush(t, mate, "same.md", "same change\n")
	if err := os.WriteFile(filepath.Join(bare, "hooks", "pre-receive"), []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := pushStore(store); err == nil {
		t.Fatal("want the hook to refuse the push")
	}
	if err := rollbackCommit(store, true, ours); err != nil {
		t.Fatal(err)
	}
	if _, err := runGitCapture(store, "cat-file", "-e", "HEAD:earlier.md"); err != nil {
		t.Error("the rollback rewound the earlier unpushed commit instead of ours")
	}
}

// The other side of the rule above: a rebase that KEEPS our commit rewrites its
// hash, and a refused retry must still undo it. This pins the assumption the
// identity match rests on, that a rebase keeps author, author date and message.
func TestRollbackAfterARebaseKeptOurCommit(t *testing.T) {
	root, bare, _ := batchStore(t)
	store, _ := Dir()
	mate := mateClone(t, root, bare)

	writeStore(t, store, "mine.md", "ours\n")
	gitT(t, store, "add", "--", "mine.md")
	gitT(t, store, "commit", "-q", "-m", "ours")
	ours, err := commitIdentity(store)
	if err != nil {
		t.Fatal(err)
	}
	before := headSHA(t, store)

	matePush(t, mate, "theirs.md", "a different change\n")
	if err := os.WriteFile(filepath.Join(bare, "hooks", "pre-receive"), []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := pushStore(store); err == nil {
		t.Fatal("want the hook to refuse the push")
	}
	if headSHA(t, store) == before {
		t.Fatal("the push path did not rebase; the test no longer covers a rewritten hash")
	}
	if err := rollbackCommit(store, true, ours); err != nil {
		t.Fatal(err)
	}
	if _, err := runGitCapture(store, "cat-file", "-e", "HEAD:mine.md"); err == nil {
		t.Error("the rebased commit was not undone")
	}
	if _, err := runGitCapture(store, "cat-file", "-e", "HEAD:theirs.md"); err != nil {
		t.Error("the undo went past our commit into the teammate's")
	}
}

// With no identity to match, rollbackCommit must not guess: it resets nothing
// and reports the commit for the user to undo.
func TestRollbackWithoutAnIdentityResetsNothing(t *testing.T) {
	_, _, _ = batchStore(t)
	store, _ := Dir()
	writeStore(t, store, "mine.md", "ours\n")
	gitT(t, store, "add", "--", "mine.md")
	gitT(t, store, "commit", "-q", "-m", "ours")
	before := headSHA(t, store)
	if err := rollbackCommit(store, true, ""); err == nil {
		t.Error("want an error naming the commit to undo by hand")
	}
	if headSHA(t, store) != before {
		t.Error("rollbackCommit reset HEAD without knowing it was our commit")
	}
}
