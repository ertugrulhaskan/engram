package team

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ertugrulhaskan/engram/internal/memory"
)

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
func Promote(memPath, placement string) (pushed bool, err error) {
	dir, err := Dir()
	if err != nil {
		return false, err
	}
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		return false, fmt.Errorf("team store not initialized — run `engram init-team <git-url>` first")
	}

	rel, err := placementPath(placement, filepath.Base(memPath))
	if err != nil {
		return false, err
	}
	dest := filepath.Join(dir, rel)

	raw, err := os.ReadFile(memPath)
	if err != nil {
		return false, err
	}
	existing, _, err := memory.ReadEngram(string(raw))
	if err != nil {
		return false, fmt.Errorf("reading engram frontmatter: %v", err)
	}
	id := existing.ID
	if id == "" {
		if id, err = memory.NewID(); err != nil {
			return false, err
		}
	}

	// Refuse to clobber a *different* memory already at this path — a filename
	// collision, most likely in the flat global/ namespace. Checked before any
	// write so a refusal leaves the local file untouched.
	if cur, err := os.ReadFile(dest); err == nil {
		if dm, ok, _ := memory.ReadEngram(string(cur)); ok && dm.ID != "" && dm.ID != id {
			return false, fmt.Errorf("a different memory is already promoted as %s — rename this one first", rel)
		}
	}

	// The sync anchor is the digest of the shared content being promoted; because
	// ContentDigest excludes the engram block, it is independent of the anchor we're
	// about to write (no circularity) and identical on the local and store copies.
	digest, err := memory.ContentDigest(string(raw))
	if err != nil {
		return false, fmt.Errorf("hashing memory content: %v", err)
	}
	owner, _ := runGitCapture(dir, "config", "user.email") // best-effort
	stamped, err := memory.WriteEngram(string(raw), memory.EngramMeta{
		ID:         id,
		Scope:      "team",
		Project:    placement,
		Owner:      owner,
		SyncedHash: digest,
	})
	if err != nil {
		return false, err
	}

	// Stamp the local file first so its engram.id is pinned and reused on any
	// retry (keeping identity stable), then write the team copy.
	if err := os.WriteFile(memPath, []byte(stamped), 0o644); err != nil {
		return false, fmt.Errorf("updating local memory: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return false, err
	}
	if err := os.WriteFile(dest, []byte(stamped), 0o644); err != nil {
		return false, fmt.Errorf("writing team copy: %v", err)
	}

	// Stage; commit only when something changed; then always push, which flushes
	// this commit and any earlier commit a prior push left behind.
	if _, err := runGitCapture(dir, "add", "--", rel); err != nil {
		return false, fmt.Errorf("staging team copy: %v", err)
	}
	// Re-promoting a previously-withdrawn id clears its tombstone, so a teammate's
	// pull won't delete the re-shared copy.
	if ledgerRel := removeWithdrawn(dir, id); ledgerRel != "" {
		if _, err := runGitCapture(dir, "add", "--", ledgerRel); err != nil {
			return false, fmt.Errorf("staging tombstone update: %v", err)
		}
	}
	if _, err := runGitCapture(dir, "diff", "--cached", "--quiet"); err != nil {
		// non-zero exit ⇒ staged changes present ⇒ commit them
		if _, err := runGitCapture(dir, "commit", "-m", "Promote "+filepath.Base(memPath)); err != nil {
			return false, fmt.Errorf("committing team copy: %v", err)
		}
	}
	if _, err := runGitCapture(dir, "push"); err != nil {
		return false, nil // commit kept locally; push failed (non-fatal)
	}
	return true, nil
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
