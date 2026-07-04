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

	byKey := make(map[string][]string, len(targets))
	for _, t := range targets {
		if t.Key != "" && t.MemoryDir != "" {
			byKey[t.Key] = append(byKey[t.Key], t.MemoryDir) // a repo cloned into >1 local dir syncs every clone
		}
	}

	projectsRoot := filepath.Join(dir, "projects")
	touched := map[string]bool{}
	localByID := map[string]map[string]string{} // memDir -> (engram id -> local path)

	// place applies one store memory to one local project dir: fast-forward when only
	// the store moved, leave a local-ahead file, flag a genuine divergence (or an
	// anchor-less memory) as a conflict, or place it fresh — never overwriting the
	// user's change. Factored out so a repo cloned into more than one local dir syncs
	// every clone, not just the last-registered one.
	place := func(memDir string, teamRaw []byte, meta memory.EngramMeta, name string) {
		ids, ok := localByID[memDir]
		if !ok {
			ids = indexByID(memDir)
			localByID[memDir] = ids
		}
		if meta.ID != "" {
			if localPath, exists := ids[meta.ID]; exists {
				localRaw, _ := os.ReadFile(localPath)
				if sameContent(string(localRaw), string(teamRaw)) {
					res.UpToDate++
					return
				}
				localMeta, _, _ := memory.ReadEngram(string(localRaw))
				switch decidePull(string(localRaw), string(teamRaw), localMeta.SyncedHash) {
				case pullUpToDate:
					res.UpToDate++
				case pullFastForward:
					if err := os.WriteFile(localPath, teamRaw, 0o644); err != nil {
						return // best-effort: leave this file for the next pull
					}
					res.Updated++
					touched[memDir] = true
				case pullLocalAhead:
					res.Ahead++ // the user's unshared local edit; leave it (↑ ahead badge)
				default: // pullConflict
					res.Conflicts++
				}
				return
			}
		}
		// New to this project — place it, unless the filename is taken by a different
		// local memory (never clobber a personal file).
		dest := filepath.Join(memDir, name)
		if _, err := os.Stat(dest); err == nil {
			res.Conflicts++
			return
		}
		if err := os.MkdirAll(memDir, 0o755); err != nil {
			return // best-effort: can't create the local dir — skip
		}
		if err := os.WriteFile(dest, teamRaw, 0o644); err != nil {
			return // best-effort: leave it for the next pull
		}
		res.Placed++
		touched[memDir] = true
	}

	walkErr := filepath.WalkDir(projectsRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if d != nil && d.IsDir() {
				return fs.SkipDir // unreadable subdir — skip it, keep pulling the rest (matches storeIndexByID)
			}
			return nil // no projects/ yet, or an unreadable entry — skip, don't abort the whole pull
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}
		rel, err := filepath.Rel(projectsRoot, path)
		if err != nil {
			return nil
		}
		key := filepath.ToSlash(filepath.Dir(rel)) // "github.com/acme/app"
		memDirs, ok := byKey[key]
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
		for _, memDir := range memDirs {
			place(memDir, teamRaw, meta, d.Name())
		}
		return nil
	})
	if walkErr != nil {
		return res, walkErr
	}

	// Propagate withdrawals: for each local team-scoped copy whose id was withdrawn
	// upstream (tombstoned) and is gone from the store, either remove a teammate's
	// copy or demote the owner's own other-checkout copy back to personal. A
	// re-promoted id is back in the store (tombstone cleared), so it is left alone.
	if withdrawn := readWithdrawn(dir); len(withdrawn) > 0 {
		storeAll := storeIndexByID(dir) // ids present anywhere in the store (any scope)
		me, _ := runGitCapture(dir, "config", "user.email")
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
					continue // keep personal copies (incl. the owner's already-withdrawn one)
				}
				// Never discard unshared work. Without an anchor we can't prove the
				// copy is unedited, so keep it rather than risk deleting local edits
				// (it surfaces as `! missing` for the user to handle). With an anchor,
				// keep it too whenever its content has moved off that anchor.
				if m.SyncedHash == "" {
					continue
				}
				if dig, err := memory.ContentDigest(string(lr)); err != nil || dig != m.SyncedHash {
					continue
				}
				// The owner's own other checkout still says scope:team (Withdraw only
				// reset the copy on the machine it ran on). Demote it to personal —
				// keeping the file — rather than deleting the owner's own memory.
				if m.Owner != "" && me != "" && m.Owner == me {
					if stamped, err := memory.WriteEngram(string(lr), memory.EngramMeta{ID: m.ID, Scope: "personal"}); err == nil {
						if os.WriteFile(localPath, []byte(stamped), 0o644) == nil {
							touched[t.MemoryDir] = true
						}
					}
					continue
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
		if containsSymlink(dir, p) {
			continue // never read *through* a symlinked entry (store hardening; mirrors storeIndexByID/promote/pull)
		}
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
