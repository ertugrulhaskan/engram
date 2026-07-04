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
	if out, err := exec.Command("git", "-C", dir, "pull", "--ff-only").CombinedOutput(); err != nil {
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

		// Already present locally (matched by id)? No-op if identical, else conflict.
		if meta.ID != "" {
			if localPath, exists := ids[meta.ID]; exists {
				localRaw, _ := os.ReadFile(localPath)
				if sameContent(string(localRaw), string(teamRaw)) {
					res.UpToDate++
				} else {
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
				if m, ok, _ := memory.ReadEngram(string(lr)); !ok || m.Scope != "team" {
					continue // keep personal copies (incl. the owner's withdrawn one)
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
