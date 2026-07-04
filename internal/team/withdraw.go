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

	// Owner guardrail: only the promoter can withdraw. Skipped when either email is
	// unknown (can't prove non-ownership).
	me, _ := runGitCapture(dir, "config", "user.email")
	if meta.Owner != "" && me != "" && meta.Owner != me {
		return false, fmt.Errorf("only %s can withdraw this memory (you are %s)", meta.Owner, me)
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
