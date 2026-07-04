package team

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/ertugrulhaskan/engram/internal/memory"
)

// ProjectTarget identifies a local project that team memories can be pulled into:
// its normalized remote Key (from ProjectKey) and its local MemoryDir.
type ProjectTarget struct {
	Key       string
	MemoryDir string
}

// PullResult summarizes a Pull.
type PullResult struct {
	Placed    int // new team memories written into a matching local project
	Updated   int // existing local copy fast-forwarded to the store (local was unchanged since sync)
	Ahead     int // local copy has unshared edits the store lacks — left as-is (↑ ahead)
	UpToDate  int // already present locally and identical
	Conflicts int // local copy differs from team — left untouched for the user to resolve
	Skipped   int // team memories whose project has no local match
	Removed   int // local team copies deleted because they were withdrawn upstream (tombstoned)
}

// Pull fetches the team store and copies project-scoped team memories into the
// matching local projects' memory dirs. It never overwrites a differing local file
// (that's a conflict for the user to resolve); identical files are no-ops; memories
// with no matching local project are skipped. Global-scoped memories are left in the
// store (browse / promote-into-a-project on demand). Touched MEMORY.md indexes are
// reconciled. Matching is by engram.id, not filename.
func Pull(targets []ProjectTarget) (PullResult, error) {
	var res PullResult
	dir, err := Dir()
	if err != nil {
		return res, err
	}
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		return res, fmt.Errorf("team store not initialized — run `engram init-team <git-url>` first")
	}
	if out, err := exec.Command("git", "-C", dir, "-c", "protocol.ext.allow=never", "pull", "--ff-only").CombinedOutput(); err != nil {
		reason := strings.TrimSpace(string(out))
		if reason == "" {
			reason = err.Error()
		}
		return res, fmt.Errorf("git pull failed: %s", reason)
	}

	byKey := make(map[string]string, len(targets))
	for _, t := range targets {
		if t.Key != "" && t.MemoryDir != "" {
			byKey[t.Key] = t.MemoryDir
		}
	}

	projectsRoot := filepath.Join(dir, "projects")
	touched := map[string]bool{}
	localByID := map[string]map[string]string{} // memDir -> (engram id -> local path)

	walkErr := filepath.WalkDir(projectsRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil // no projects/ yet — nothing to pull
			}
			return err
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}
		rel, err := filepath.Rel(projectsRoot, path)
		if err != nil {
			return nil
		}
		key := filepath.ToSlash(filepath.Dir(rel)) // "github.com/acme/app"
		memDir, ok := byKey[key]
		if !ok {
			res.Skipped++
			return nil
		}

		if containsSymlink(dir, path) {
			return nil // never read *through* a symlink a teammate committed into the store
		}
		teamRaw, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		meta, _, _ := memory.ReadEngram(string(teamRaw))

		ids, ok := localByID[memDir]
		if !ok {
			ids = indexByID(memDir)
			localByID[memDir] = ids
		}

		// Already present locally (matched by id)? Identical is a no-op; otherwise
		// consult the anchor: fast-forward when only the store moved, leave a
		// local-ahead file alone, and treat a genuine divergence (or an anchor-less
		// memory) as a conflict — never overwriting the user's change.
		if meta.ID != "" {
			if localPath, exists := ids[meta.ID]; exists {
				localRaw, _ := os.ReadFile(localPath)
				if sameContent(string(localRaw), string(teamRaw)) {
					res.UpToDate++
					return nil
				}
				localMeta, _, _ := memory.ReadEngram(string(localRaw))
				switch decidePull(string(localRaw), string(teamRaw), localMeta.SyncedHash) {
				case pullUpToDate:
					res.UpToDate++
				case pullFastForward:
					if err := os.WriteFile(localPath, teamRaw, 0o644); err != nil {
						return err
					}
					res.Updated++
					touched[memDir] = true
				case pullLocalAhead:
					res.Ahead++ // the user's unshared local edit; leave it (↑ ahead badge)
				default: // pullConflict
					res.Conflicts++
				}
				return nil
			}
		}

		// New to this project — place it, unless the filename is taken by a
		// different local memory (never clobber a personal file).
		dest := filepath.Join(memDir, d.Name())
		if _, err := os.Stat(dest); err == nil {
			res.Conflicts++
			return nil
		}
		if err := os.MkdirAll(memDir, 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dest, teamRaw, 0o644); err != nil {
			return err
		}
		res.Placed++
		touched[memDir] = true
		return nil
	})
	if walkErr != nil {
		return res, walkErr
	}

	// Propagate withdrawals: delete local team-scoped copies whose id was withdrawn
	// upstream (tombstoned) and is no longer in the store. A re-promoted id is back
	// in the store (and its tombstone cleared), so it is not removed. The owner's
	// own copy was reset to personal by Withdraw, so it is skipped here.
	if withdrawn := readWithdrawn(dir); len(withdrawn) > 0 {
		storeAll := storeIndexByID(dir) // ids present anywhere in the store (any scope)
		for _, t := range targets {
			if t.Key == "" || t.MemoryDir == "" {
				continue
			}
			for id, localPath := range indexByID(t.MemoryDir) {
				if !withdrawn[id] || len(storeAll[id]) > 0 {
					continue // not withdrawn, or still shared somewhere in the store
				}
				lr, err := os.ReadFile(localPath)
				if err != nil {
					continue
				}
				m, ok, _ := memory.ReadEngram(string(lr))
				if !ok || m.Scope != "team" {
					continue // keep personal copies (incl. the owner's withdrawn one)
				}
				// Never delete a copy the user has edited since it last synced — that
				// would silently discard unshared work. With an anchor present, keep
				// the file whenever its content no longer matches it (it shows
				// `! missing` for the user to handle). An anchor-less legacy copy
				// can't be checked, so it keeps the prior behavior.
				if m.SyncedHash != "" {
					if dig, err := memory.ContentDigest(string(lr)); err != nil || dig != m.SyncedHash {
						continue
					}
				}
				if os.Remove(localPath) == nil {
					res.Removed++
					touched[t.MemoryDir] = true
				}
			}
		}
	}

	for memDir := range touched {
		_, _, _ = memory.ReconcileIndex(memDir) // refresh the index; best-effort
	}
	return res, nil
}

// pullAction is what Pull should do with a local team memory that differs from
// its store copy, decided from the local file's recorded base anchor.
type pullAction int

const (
	pullConflict    pullAction = iota // both sides moved, or no anchor to tell — never overwrite
	pullUpToDate                      // same shared content; only engram bookkeeping differs
	pullFastForward                   // only the store moved — safe to take the store version
	pullLocalAhead                    // only local moved — the user's unshared work; leave it
)

// decidePull classifies a differing (local, store) pair using base — the digest of
// the content the local copy was last synced to — by mapping the shared relationOf
// rule onto pull actions. It never proposes overwriting a local change: without an
// anchor, or when both sides moved, it stays a conflict.
func decidePull(localRaw, teamRaw, base string) pullAction {
	lh, err1 := memory.ContentDigest(localRaw)
	sh, err2 := memory.ContentDigest(teamRaw)
	if err1 != nil || err2 != nil {
		return pullConflict // can't hash — don't risk clobbering the local file
	}
	switch relationOf(lh, base, map[string]bool{sh: true}) {
	case StateSynced:
		return pullUpToDate // shared content already matches; only bookkeeping differs
	case StateIncoming:
		return pullFastForward // local at base, store advanced → safe to take
	case StateLocalAhead:
		return pullLocalAhead // local advanced, store at base → leave the user's work
	default: // StateDiverged (both moved) or StateDiffers (no anchor)
		return pullConflict
	}
}

// sameContent compares two memory files ignoring line-ending differences, so a
// CRLF/LF mismatch (e.g. git autocrlf normalizing one side) doesn't read as a
// spurious conflict.
func sameContent(a, b string) bool {
	return strings.ReplaceAll(a, "\r\n", "\n") == strings.ReplaceAll(b, "\r\n", "\n")
}

// indexByID maps engram.id -> file path for the memory files in dir.
func indexByID(dir string) map[string]string {
	out := map[string]string{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return out
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		p := filepath.Join(dir, e.Name())
		raw, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		if m, ok, _ := memory.ReadEngram(string(raw)); ok && m.ID != "" {
			out[m.ID] = p
		}
	}
	return out
}
