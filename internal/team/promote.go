package team

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ertugrulhaskan/engram/internal/memory"
)

// PromoteItem is one memory in a batch promote: the local memory path, and the
// placement it is headed to ("global" or a normalized project key from
// ProjectKey). Each item carries its own placement, so a single batch can span
// several projects — which memory belongs where is the caller's call, not the
// store's.
type PromoteItem struct {
	Path      string
	Placement string
}

// Promote copies a personal memory into the team store and marks it shared. The
// memory at memPath is stamped with an `engram:` block (a fresh id if it lacks one,
// scope=team, the given placement, and the promoter's git email); the same stamped
// content is written into the clone under global/ or projects/<key>/, then committed
// and pushed. placement is "global" or a normalized project key from ProjectKey.
// Re-promoting unchanged content is a no-op.
//
// All git runs through captured output (no terminal takeover) so Promote is safe to
// call from inside the TUI. pushed reports whether the commit reached the remote: a
// non-interactive push that fails (missing creds/remote) is non-fatal — the local
// commit is kept and pushed=false, leaving the caller to surface a retry hint.
//
// This is the one-memory spelling of PromoteBatch, which carries the mechanics.
func Promote(memPath, placement string) (pushed bool, err error) {
	return PromoteBatch([]PromoteItem{{Path: memPath, Placement: placement}})
}

// PromoteBatch promotes several memories as a single unit of history: every item
// is stamped and written, all of it is staged together, and one commit and one
// push cover the whole batch. Promoting N memories one at a time would produce N
// commits and N pushes for what the user performed as one act.
//
// Nothing is written until every item has been prepared, so a batch that cannot
// go through — an unsafe key, an unreadable memory, a filename that collides in
// the store or against another item — fails with every local file untouched
// rather than half-applied.
func PromoteBatch(items []PromoteItem) (pushed bool, err error) {
	if len(items) == 0 {
		return false, fmt.Errorf("nothing to promote")
	}
	dir, err := Dir()
	if err != nil {
		return false, err
	}
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		return false, fmt.Errorf("team store not initialized — run `engram init-team <git-url>` first")
	}

	// One identity lookup for the whole batch: every copy is promoted by the same
	// person in the same act, and it saves a git shell-out per memory.
	owner, _ := runGitCapture(dir, "config", "user.email") // best-effort

	prepared := make([]preparedPromotion, 0, len(items))
	claimed := make(map[string]string, len(items)) // folded store path -> the memory taking it
	for _, it := range items {
		p, err := preparePromotion(dir, it, owner)
		if err != nil {
			return false, err
		}
		// Two memories in one batch landing on the same store path would leave the
		// second silently overwriting the first. Against what is already on disk
		// that is the collision check in preparePromotion; within a batch neither
		// copy exists yet, so it has to be caught here.
		//
		// Compared case-folded, because on a case-insensitive filesystem (macOS,
		// Windows) "Notes.md" and "notes.md" ARE one file, and an exact compare
		// would wave the pair through and lose one of them. Folding on a
		// case-sensitive filesystem costs a refusal for a pair that differs only in
		// case, which is the right trade: such a pair cannot survive in a store any
		// teammate clones onto a case-insensitive disk.
		fold := strings.ToLower(p.rel)
		if first, dup := claimed[fold]; dup {
			return false, fmt.Errorf("%s and %s would both promote as %s — rename one first",
				filepath.Base(first), filepath.Base(p.memPath), p.rel)
		}
		claimed[fold] = p.memPath
		prepared = append(prepared, p)
	}

	for _, p := range prepared {
		// Stamp the local file first so its engram.id is pinned and reused on any
		// retry (keeping identity stable), then write the team copy.
		if err := os.WriteFile(p.memPath, []byte(p.stamped), 0o644); err != nil {
			return false, fmt.Errorf("updating local memory: %v", err)
		}
		if err := os.MkdirAll(filepath.Dir(p.dest), 0o755); err != nil {
			return false, err
		}
		if err := os.WriteFile(p.dest, []byte(p.stamped), 0o644); err != nil {
			return false, fmt.Errorf("writing team copy: %v", err)
		}
		if _, err := runGitCapture(dir, "add", "--", p.rel); err != nil {
			return false, fmt.Errorf("staging team copy: %v", err)
		}
		// Re-promoting a previously-withdrawn id clears its tombstone, so a teammate's
		// pull won't delete the re-shared copy.
		if ledgerRel := removeWithdrawn(dir, p.id); ledgerRel != "" {
			if _, err := runGitCapture(dir, "add", "--", ledgerRel); err != nil {
				return false, fmt.Errorf("staging tombstone update: %v", err)
			}
		}
	}

	// Commit only when something changed; then always push, which flushes this
	// commit and any earlier commit a prior push left behind.
	if _, err := runGitCapture(dir, "diff", "--cached", "--quiet"); err != nil {
		// non-zero exit ⇒ staged changes present ⇒ commit them
		if _, err := runGitCapture(dir, "commit", "-m", promoteMessage(prepared)); err != nil {
			return false, fmt.Errorf("committing team copy: %v", err)
		}
	}
	if _, err := runGitCapture(dir, "push"); err != nil {
		return false, nil // commit kept locally; push failed (non-fatal)
	}
	return true, nil
}

// preparedPromotion is one item resolved and stamped, ready to be written. The
// preparation pass does every check that can fail before the first byte is
// written, which is what lets a rejected batch leave nothing behind.
type preparedPromotion struct {
	memPath string // the local memory
	rel     string // its path inside the store, relative to the clone root
	dest    string // that same path, absolute
	id      string // the memory's engram id (freshly minted when it had none)
	stamped string // the content both copies receive
}

// preparePromotion resolves one item's destination and computes its stamped
// content. It writes nothing — every refusal here costs the caller no cleanup.
func preparePromotion(dir string, it PromoteItem, owner string) (preparedPromotion, error) {
	var p preparedPromotion

	rel, err := placementPath(it.Placement, filepath.Base(it.Path))
	if err != nil {
		return p, err
	}
	dest := filepath.Join(dir, rel)

	// Refuse to act on a destination that traverses a symlink in the store — a
	// teammate could commit one pointing outside the store (e.g. at ~/.ssh or a
	// shell rc), turning the read/write below into an arbitrary-file access.
	if containsSymlink(dir, dest) {
		return p, fmt.Errorf("refusing to promote through a symlink in the team store")
	}

	raw, err := os.ReadFile(it.Path)
	if err != nil {
		return p, err
	}
	existing, _, err := memory.ReadEngram(string(raw))
	if err != nil {
		return p, fmt.Errorf("reading engram frontmatter: %v", err)
	}
	id := existing.ID
	if id == "" {
		if id, err = memory.NewID(); err != nil {
			return p, err
		}
	}

	// Refuse to clobber a *different* memory already at this path — a filename
	// collision, most likely in the flat global/ namespace. Checked before any
	// write so a refusal leaves the local file untouched.
	if cur, err := os.ReadFile(dest); err == nil {
		if dm, ok, _ := memory.ReadEngram(string(cur)); ok && dm.ID != "" && dm.ID != id {
			return p, fmt.Errorf("a different memory is already promoted as %s — rename this one first", rel)
		}
	}

	// The sync anchor is the digest of the shared content being promoted; because
	// ContentDigest excludes the engram block, it is independent of the anchor we're
	// about to write (no circularity) and identical on the local and store copies.
	digest, err := memory.ContentDigest(string(raw))
	if err != nil {
		return p, fmt.Errorf("hashing memory content: %v", err)
	}
	stamped, err := memory.WriteEngram(string(raw), memory.EngramMeta{
		ID:         id,
		Scope:      "team",
		Project:    it.Placement,
		Owner:      owner,
		SyncedHash: digest,
	})
	if err != nil {
		return p, err
	}
	return preparedPromotion{memPath: it.Path, rel: rel, dest: dest, id: id, stamped: stamped}, nil
}

// promoteMessage names the commit. A single promote keeps its filename, which is
// what the store's history has carried since the first release; a batch counts
// instead, since a list of names would run well past a readable subject line.
func promoteMessage(ps []preparedPromotion) string {
	if len(ps) == 1 {
		return "Promote " + filepath.Base(ps[0].memPath)
	}
	return fmt.Sprintf("Promote %d memories", len(ps))
}

// placementPath maps a placement ("global" or a normalized project key) and a
// filename to a path inside the team store, guarding against traversal — the
// project key comes from NormalizeRemote, which does not strip "..".
func placementPath(placement, filename string) (string, error) {
	if placement == "global" {
		return filepath.Join("global", filename), nil
	}
	clean := filepath.ToSlash(filepath.Clean(placement))
	if clean == "" || clean == "." || clean == ".." ||
		strings.HasPrefix(clean, "/") || strings.HasPrefix(clean, "../") || strings.Contains(clean, "/../") {
		return "", fmt.Errorf("unsafe project key: %q", placement)
	}
	return filepath.Join("projects", clean, filename), nil
}
