package team

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ErrStoreDiverged reports that the store clone and its remote both moved and
// git could not rebase one onto the other — a real content conflict, most often
// two people changing the same store memory at once. engram aborts the rebase,
// so the clone is left clean with the local commit kept, and hands off to the
// user (SPEC §7). It is never a credentials problem, so callers must not say so.
var ErrStoreDiverged = errors.New("the team store and its remote both changed the same file")

// pushStore pushes the store clone's commits. When the push is rejected because
// a teammate pushed first, it rebases the local commits onto the remote and
// retries once — what SPEC §7 promises. Without that, one rejected push left a
// local commit that every later `pull --ff-only` refused to move past, wedging
// the store until someone fixed it with git by hand.
//
// It returns pushed=false with a nil error when the remote cannot be reached
// (offline, missing creds): the commits stay local and the next push or pull
// flushes them. A rebase conflict aborts the rebase and returns ErrStoreDiverged.
func pushStore(dir string) (bool, error) {
	out, err := runGitCombined(dir, "push")
	if err == nil {
		return true, nil
	}
	// Rejected, or unreachable? Only a fetch tells them apart.
	if _, err := runGitCapture(dir, "-c", "protocol.ext.allow=never", "fetch"); err != nil {
		return false, nil // unreachable: keep the commit, flush later
	}
	behind, err := runGitCapture(dir, "rev-list", "--count", "HEAD..@{u}")
	if err != nil {
		// No upstream to compare against: a local setup problem, not the server.
		return false, fmt.Errorf("the team store has no upstream branch to push to — check it with git in %s", dir)
	}
	if behind == "0" {
		// The remote is reachable and did not move, yet refused the push: a
		// server-side hook, branch protection, or no write access (or no
		// credentials for pushing). Retrying will never help, so say what git
		// said rather than "offline / check creds".
		return false, pushRefused(out)
	}
	if err := rebaseOntoUpstream(dir); err != nil {
		return false, err
	}
	if out, err := runGitCombined(dir, "push"); err != nil {
		return false, pushRefused(out)
	}
	return true, nil
}

// pushRefused names a push the remote refused, with git's own reason: the
// "remote:", "error:" or "rejected" lines, else the last line it printed.
func pushRefused(out string) error {
	var reasons []string
	lines := strings.Split(strings.TrimSpace(out), "\n")
	for _, ln := range lines {
		t := strings.TrimSpace(ln)
		if strings.HasPrefix(t, "remote:") || strings.HasPrefix(t, "error:") || strings.Contains(t, "rejected") {
			reasons = append(reasons, t)
		}
	}
	if len(reasons) == 0 && len(lines) > 0 {
		reasons = append(reasons, strings.TrimSpace(lines[len(lines)-1]))
	}
	return fmt.Errorf("the team remote didn't accept the push: %s", strings.Join(reasons, " · "))
}

// syncStore brings the store clone up to its remote before a read or a plan:
// local commits a failed push left behind are rebased onto the remote rather
// than wedging a fast-forward-only pull. It never pushes — PullPlan is a preview
// — and --autostash carries any uncommitted leftover (a promote that failed
// partway through staging) across the rebase instead of refusing the pull. A
// clean clone just fast-forwards, as before.
func syncStore(dir string) error {
	out, err := runGitCombined(dir, "-c", "protocol.ext.allow=never", "pull", "--rebase", "--autostash")
	if err != nil {
		if rebaseInProgress(dir) {
			return finishOrAbortRebase(dir)
		}
		// Not a rebase stop, yet files are unmerged: the autostash didn't
		// re-apply cleanly. That is a conflict, not "offline" — callers must stop,
		// not carry on and commit conflict markers.
		if u, _ := runGitCapture(dir, "diff", "--name-only", "--diff-filter=U"); strings.TrimSpace(u) != "" {
			return storeDivergedError(dir)
		}
		reason := strings.TrimSpace(out)
		if reason == "" {
			reason = err.Error()
		}
		return fmt.Errorf("git pull failed: %s", reason)
	}
	return nil
}

// flushStore pushes commits the clone holds that its upstream lacks — what a
// failed push left behind. Best-effort: a failure leaves them for next time.
func flushStore(dir string) {
	if ahead, err := runGitCapture(dir, "rev-list", "--count", "@{u}..HEAD"); err == nil && ahead != "0" {
		_, _ = pushStore(dir)
	}
}

// rebaseOntoUpstream replays the clone's local commits onto its fetched
// upstream. A stop whose only conflict is the tombstone ledger is resolved by
// resolveLedgerOnlyConflicts; any other conflict aborts, so the clone is never
// left mid-rebase.
func rebaseOntoUpstream(dir string) error {
	if _, err := runGitCapture(dir, "rebase", "@{u}"); err != nil {
		if rebaseInProgress(dir) {
			return finishOrAbortRebase(dir)
		}
		return storeDivergedError(dir)
	}
	return nil
}

// finishOrAbortRebase is the one policy for a stopped rebase, shared by the
// pull and the push paths: finish it when every stop is a ledger-only conflict
// (resolveLedgerOnlyConflicts), otherwise abort — the clone is never left
// mid-rebase — and report the divergence.
func finishOrAbortRebase(dir string) error {
	if err := resolveLedgerOnlyConflicts(dir); err == nil {
		return nil
	}
	_, _ = runGitCapture(dir, "rebase", "--abort")
	return storeDivergedError(dir)
}

// resolveLedgerOnlyConflicts drives a stopped rebase to completion while the
// only conflicted file at each stop is the tombstone ledger, merging it by id
// (mergeLedger). Two people withdrawing at once, or one re-promoting while
// another withdraws, both edit the end of that file — a textual conflict with no
// real disagreement. Any other conflicted file is a real one: it returns an error
// and the caller aborts.
func resolveLedgerOnlyConflicts(dir string) error {
	for i := 0; i < 100 && rebaseInProgress(dir); i++ {
		files, err := runGitCapture(dir, "diff", "--name-only", "--diff-filter=U")
		if err != nil {
			return err
		}
		if strings.TrimSpace(files) != withdrawnLedger {
			return errors.New("conflict outside the tombstone ledger")
		}
		if ledgerIsSymlink(dir) {
			return errors.New("the tombstone ledger is a symlink") // never written through one
		}
		merged := mergeLedger(gitShow(dir, ":1:"+withdrawnLedger),
			gitShow(dir, ":2:"+withdrawnLedger), gitShow(dir, ":3:"+withdrawnLedger))
		if err := os.WriteFile(filepath.Join(dir, withdrawnLedger), []byte(merged), 0o644); err != nil {
			return err
		}
		if _, err := runGitCapture(dir, "add", "--", withdrawnLedger); err != nil {
			return err
		}
		cont := quietGit("-C", dir, "-c", "core.editor=true", "rebase", "--continue")
		cont.Env = append(cont.Env, "GIT_EDITOR=true")
		if err := cont.Run(); err != nil && !rebaseInProgress(dir) {
			return err
		}
	}
	if rebaseInProgress(dir) {
		return errors.New("the rebase did not finish")
	}
	return nil
}

// mergeLedger is a three-way merge of the tombstone ledger by id. An id stays
// when both sides keep it, or when either side added it; it goes when either
// side removed it. Git's "union" driver was the first answer and is wrong here:
// it keeps every line from both sides, so a tombstone one side deliberately
// removed (a re-promote) came back — and the next pull deleted the re-shared
// memory from every teammate. Comment and blank lines follow side a.
func mergeLedger(base, a, b string) string {
	baseIDs := ledgerIDs(base)
	aIDs, bIDs := ledgerIDs(a), ledgerIDs(b)
	keep := func(id string) bool {
		return (aIDs[id] && bIDs[id]) || (!baseIDs[id] && (aIDs[id] || bIDs[id]))
	}
	var out []string
	written := map[string]bool{}
	// Comment and blank lines come from side a, or from b when a is empty (a
	// deleted or never-created side), so an empty side adds no stray blank line
	// and the other side's header isn't lost.
	commentSide := a
	if strings.TrimSpace(a) == "" {
		commentSide = b
	}
	for _, side := range []string{a, b} {
		if side == "" {
			continue
		}
		for _, ln := range strings.Split(strings.TrimRight(side, "\n"), "\n") {
			t := strings.TrimSpace(ln)
			if t == "" || strings.HasPrefix(t, "#") {
				if side == commentSide {
					out = append(out, ln)
				}
				continue
			}
			id := strings.Fields(t)[0]
			if keep(id) && !written[id] {
				written[id] = true
				out = append(out, ln)
			}
		}
	}
	return strings.Join(out, "\n") + "\n"
}

func ledgerIDs(s string) map[string]bool {
	out := map[string]bool{}
	for _, ln := range strings.Split(s, "\n") {
		if t := strings.TrimSpace(ln); t != "" && !strings.HasPrefix(t, "#") {
			out[strings.Fields(t)[0]] = true
		}
	}
	return out
}

// gitShow returns a blob's raw content ("" when absent, e.g. no common
// ancestor for a file both sides created).
func gitShow(dir, spec string) string {
	out, err := exec.Command("git", "-C", dir, "show", spec).Output()
	if err != nil {
		return ""
	}
	return string(out)
}

func storeDivergedError(dir string) error {
	return fmt.Errorf("%w — resolve it with git in %s, then push", ErrStoreDiverged, dir)
}

func rebaseInProgress(dir string) bool {
	for _, d := range []string{"rebase-merge", "rebase-apply"} {
		if _, err := os.Stat(filepath.Join(dir, ".git", d)); err == nil {
			return true
		}
	}
	return false
}

// rollbackCommit undoes the commit the caller just made (when it made one),
// with --keep so any other uncommitted work in the clone is left alone. Used
// when a push is refused or diverged — outcomes a retry can't fix — so the
// clone doesn't keep a commit whose effect never reached the team.
//
// ours is commitIdentity read right after that commit. pushStore's rebase
// rewrites the commit's hash but keeps its author, author date and message, so
// the identity still finds it there. When the rebase dropped it as empty (a
// teammate pushed the same change), HEAD is a different commit, such as an
// earlier one an offline push left for later, and resetting would rewind that
// one instead; so nothing is reset. An empty ours (the identity couldn't be
// read after the commit) can't tell the two apart, so it resets nothing and
// reports the commit for the user to undo, rather than guess.
func rollbackCommit(dir string, committed bool, ours string) error {
	if !committed {
		return nil
	}
	failed := fmt.Errorf("undoing the local commit failed — reset it with git in %s", dir)
	if ours == "" {
		return failed
	}
	head, err := commitIdentity(dir)
	if err != nil {
		return failed
	}
	if head != ours {
		return nil // the rebase dropped our commit; HEAD is not ours to undo
	}
	if _, err := runGitCapture(dir, "reset", "--keep", "HEAD~1"); err != nil {
		return failed
	}
	return nil
}

// commitIdentity is HEAD's author, author date and message: what a rebase keeps
// when it rewrites a commit's hash (rollbackCommit).
func commitIdentity(dir string) (string, error) {
	return runGitCapture(dir, "log", "-1", "--date=raw", "--format=%an%x00%ae%x00%ad%x00%B", "HEAD")
}

// restorePaths puts the given store paths back to HEAD in both the index and the
// working tree — unstaging what a failed promote or withdraw staged, restoring
// files it removed, and deleting files it created. Without it, a call that
// failed before committing left staged changes behind for the NEXT, unrelated
// commit to sweep in and push (a withdraw's staged deletion could remove a team
// memory for everyone). Best-effort.
func restorePaths(dir string, rels []string) {
	if len(rels) == 0 {
		return
	}
	_, _ = runGitCapture(dir, append([]string{"reset", "-q", "--"}, rels...)...)
	for _, rel := range rels {
		if _, err := runGitCapture(dir, "cat-file", "-e", "HEAD:"+filepath.ToSlash(rel)); err == nil {
			_, _ = runGitCapture(dir, "checkout", "-q", "HEAD", "--", rel)
		} else {
			_ = os.Remove(filepath.Join(dir, rel))
		}
	}
}
