package team

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/ertugrulhaskan/engram/internal/memory"
)

// Withdraw is the reverse of Promote: it removes a memory's shared copy from the
// team store, resets the local memory's scope back to personal, and commits +
// pushes the removal. The local file keeps its engram.id, so a later re-promote
// reuses the same identity. Teammates who already pulled the memory keep their
// local copy (pull never deletes) — withdraw stops future sharing, it does not
// recall existing copies.
//
// removed reports whether a store copy was found and deleted; pushed reports
// whether that deletion reached the remote. A memory that isn't in the store is
// still reset to personal (removed=false, pushed=false, err=nil).
func Withdraw(memPath string) (removed, pushed bool, err error) {
	dir, err := Dir()
	if err != nil {
		return false, false, err
	}
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		return false, false, fmt.Errorf("team store not initialized — run `engram init-team <git-url>` first")
	}

	raw, err := os.ReadFile(memPath)
	if err != nil {
		return false, false, err
	}
	meta, ok, err := memory.ReadEngram(string(raw))
	if err != nil {
		return false, false, fmt.Errorf("reading engram frontmatter: %v", err)
	}
	if !ok || meta.Scope != "team" || meta.ID == "" {
		return false, false, fmt.Errorf("this memory isn't shared with the team")
	}

	// Find the store copy by engram.id within its scope dir (robust to a local
	// rename, since the store filename could differ from the current local one).
	scopeDir := filepath.Join(dir, "global")
	if meta.Project != "global" {
		rel, err := placementPath(meta.Project, "_") // reuse placementPath's traversal guard
		if err != nil {
			return false, false, err
		}
		scopeDir = filepath.Join(dir, filepath.Dir(rel))
	}
	storePath := indexByID(scopeDir)[meta.ID]

	// Remove the store copy (commit + push) when it's present. A missing copy is
	// fine — the memory may have been withdrawn already or never reached the remote.
	if storePath != "" {
		relPath, err := filepath.Rel(dir, storePath)
		if err != nil {
			return false, false, err
		}
		// Delete the file and stage the removal with `add -A` (rather than `git rm`)
		// so a copy left untracked or staged by an interrupted promote is handled too.
		if err := os.Remove(storePath); err != nil {
			return false, false, fmt.Errorf("removing team copy: %v", err)
		}
		removed = true
		if _, err := runGitCapture(dir, "add", "-A", "--", relPath); err != nil {
			return removed, false, fmt.Errorf("staging removal: %v", err)
		}
		// Commit + push only when the removal changed the index; a copy that was
		// never committed leaves nothing to push (it was never on the remote).
		if _, err := runGitCapture(dir, "diff", "--cached", "--quiet"); err != nil {
			if _, err := runGitCapture(dir, "commit", "-m", "Withdraw "+filepath.Base(storePath)); err != nil {
				return removed, false, fmt.Errorf("committing withdrawal: %v", err)
			}
			if _, err := runGitCapture(dir, "push"); err == nil {
				pushed = true
			}
		} else {
			pushed = true // nothing to commit/push — the copy was never shared
		}
	}

	// Reset the local memory to personal, keeping its id for a possible re-promote.
	stamped, err := memory.WriteEngram(string(raw), memory.EngramMeta{ID: meta.ID, Scope: "personal"})
	if err != nil {
		return removed, pushed, err
	}
	if err := os.WriteFile(memPath, []byte(stamped), 0o644); err != nil {
		return removed, pushed, fmt.Errorf("updating local memory: %v", err)
	}
	return removed, pushed, nil
}
