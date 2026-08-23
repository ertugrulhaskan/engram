package team

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/ertugrulhaskan/engram/internal/memory"
)

// Withdraw is the reverse of Promote: it removes a memory's shared copy from the
// team store, records the id in the withdrawn ledger (a tombstone, so a teammate's
// pull removes their copy too), and resets the local memory's scope back to
// personal — keeping its id so a later re-promote reuses the same identity and
// clears the tombstone.
//
// Only the memory's owner may withdraw it: engram compares the stored `owner`
// (the promoter's git email) to the current user's. This is an accident guardrail,
// not enforcement — anyone with push access to the store can bypass it.
//
// pushed reports whether the removal reached the remote.
func Withdraw(memPath string) (pushed bool, err error) {
	dir, err := Dir()
	if err != nil {
		return false, err
	}
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		return false, fmt.Errorf("team store not initialized — run `engram init-team <git-url>` first")
	}

	raw, err := os.ReadFile(memPath)
	if err != nil {
		return false, err
	}
	meta, ok, err := memory.ReadEngram(string(raw))
	if err != nil {
		return false, fmt.Errorf("reading engram frontmatter: %v", err)
	}
	if !ok || meta.Scope != "team" || meta.ID == "" {
		return false, fmt.Errorf("this memory isn't shared with the team")
	}

	// Owner guardrail: only the promoter can withdraw. Fails open when either side
	// is unknown — engram can't prove non-ownership, and refusing to withdraw your
	// own memory because git is misconfigured would be the worse failure. The
	// confirm dialog says the check was skipped, and why, before reaching here.
	me, _ := runGitCapture(dir, "config", "user.email")
	if own := (OwnerStatus{Owner: meta.Owner, Me: me}); own.Verifiable() && !own.Mine() {
		return false, fmt.Errorf("only %s can withdraw this memory (you are %s)", meta.Owner, me)
	}

	// Pre-flight: confirm we can rewrite the local file back to personal BEFORE we
	// remove anything from the store. Otherwise the store removal would commit + push
	// and then the local reset could fail (e.g. frontmatter engram can't safely edit),
	// leaving the memory "shared but no longer in the store" — the `! missing` state.
	// Refusing here keeps the memory fully shared and untouched instead.
	if _, err := memory.WriteEngram(string(raw), memory.EngramMeta{ID: meta.ID, Scope: "personal"}); err != nil {
		return false, fmt.Errorf("can't withdraw — this memory's frontmatter can't be safely updated: %v", err)
	}

	// Find the store copy by engram.id within its scope dir (robust to a local
	// rename, since the store filename could differ from the current local one).
	scopeDir := filepath.Join(dir, "global")
	if meta.Project != "global" {
		rel, err := placementPath(meta.Project, "_") // reuse placementPath's traversal guard
		if err != nil {
			return false, err
		}
		scopeDir = filepath.Join(dir, filepath.Dir(rel))
	}

	// Delete the store copy (if present) and tombstone the id. Both are staged and
	// committed together, so pull sees the removal + the tombstone atomically.
	if storePath := indexByID(scopeDir)[meta.ID]; storePath != "" {
		relPath, err := filepath.Rel(dir, storePath)
		if err != nil {
			return false, err
		}
		if err := os.Remove(storePath); err != nil {
			return false, fmt.Errorf("removing team copy: %v", err)
		}
		if _, err := runGitCapture(dir, "add", "-A", "--", relPath); err != nil {
			return false, fmt.Errorf("staging removal: %v", err)
		}
	}
	if ledgerRel := addWithdrawn(dir, meta.ID, filepath.Base(memPath)); ledgerRel != "" {
		if _, err := runGitCapture(dir, "add", "--", ledgerRel); err != nil {
			return false, fmt.Errorf("staging tombstone: %v", err)
		}
	}

	// Commit + push only when something changed; a memory that was already
	// withdrawn leaves nothing to do.
	if _, err := runGitCapture(dir, "diff", "--cached", "--quiet"); err != nil {
		if _, err := runGitCapture(dir, "commit", "-m", "Withdraw "+filepath.Base(memPath)); err != nil {
			return false, fmt.Errorf("committing withdrawal: %v", err)
		}
		if _, err := runGitCapture(dir, "push"); err == nil {
			pushed = true
		}
	} else {
		pushed = true // nothing staged — already withdrawn
	}

	// Reset the local memory to personal, keeping its id for a possible re-promote.
	// Re-read the file here rather than reusing the copy read before the (possibly
	// slow) push, so an edit made to the memory while the push was in flight isn't
	// clobbered with stale content — only the scope flips, the current body stays.
	fresh, err := os.ReadFile(memPath)
	if err != nil {
		return pushed, fmt.Errorf("re-reading local memory: %v", err)
	}
	stamped, err := memory.WriteEngram(string(fresh), memory.EngramMeta{ID: meta.ID, Scope: "personal"})
	if err != nil {
		return pushed, err
	}
	if err := os.WriteFile(memPath, []byte(stamped), 0o644); err != nil {
		return pushed, fmt.Errorf("updating local memory: %v", err)
	}
	return pushed, nil
}

// OwnerStatus reports the two inputs to withdraw's owner guardrail, so a caller can
// tell the user when the comparison could not be made at all — and which side was
// missing. The two causes are genuinely different: an empty Owner is a property of
// the memory (promoted before engram recorded owners, or by a machine with no git
// email), while an empty Me is a property of this machine.
type OwnerStatus struct {
	Owner string // the promoter's git email, recorded at promote time; "" if the memory records none
	Me    string // this machine's git user.email; "" if unset
}

// Verifiable reports whether both sides are known, so ownership can be compared.
func (o OwnerStatus) Verifiable() bool { return o.Owner != "" && o.Me != "" }

// Mine reports a positively verified match — this machine promoted the memory.
func (o OwnerStatus) Mine() bool { return o.Verifiable() && o.Owner == o.Me }

// CheckOwner reports whether the owner guardrail can be evaluated for a memory,
// without withdrawing anything. It exists so the confirm dialog can say the check
// was skipped before the user commits to it; Withdraw applies the same comparison
// itself, so the two can't drift.
func CheckOwner(memPath string) (OwnerStatus, error) {
	dir, err := Dir()
	if err != nil {
		return OwnerStatus{}, err
	}
	raw, err := os.ReadFile(memPath)
	if err != nil {
		return OwnerStatus{}, err
	}
	meta, ok, err := memory.ReadEngram(string(raw))
	if err != nil {
		return OwnerStatus{}, fmt.Errorf("reading engram frontmatter: %v", err)
	}
	// A failed git read leaves Me empty, which is the unverifiable case the caller
	// is asking about — not an error worth refusing the withdraw over.
	me, _ := runGitCapture(dir, "config", "user.email")
	if !ok {
		return OwnerStatus{Me: me}, nil
	}
	return OwnerStatus{Owner: meta.Owner, Me: me}, nil
}
