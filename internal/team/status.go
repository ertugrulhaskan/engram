package team

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/ertugrulhaskan/engram/internal/memory"
)

// SyncState is a local memory's relationship to the shared team store.
type SyncState int

const (
	StateNone    SyncState = iota // not shared (personal, or no engram block)
	StateSynced                   // scope=team, present in the store, content identical
	StateDiffers                  // scope=team, present in the store, content differs
	StateMissing                  // scope=team, but no matching id in the store
)

// SyncStates reports each memory's relationship to the team store, keyed by the
// memory's Path. It is read-only and best-effort: when no team store is
// initialized it returns an empty map, so a caller can treat any missing key as
// StateNone. Only team-scoped memories get an entry; personal/unstamped ones map
// to StateNone. Matching is by engram.id, never by filename — mirroring Pull, and
// the content compare is line-ending tolerant (sameContent).
func SyncStates(mems []memory.Memory) (map[string]SyncState, error) {
	out := make(map[string]SyncState, len(mems))
	if !IsInitialized() {
		return out, nil
	}
	dir, err := Dir()
	if err != nil {
		return out, err
	}
	store := storeIndexByID(dir)
	for _, mm := range mems {
		meta, ok, err := memory.ReadEngram(mm.Raw)
		if err != nil || !ok || meta.Scope != "team" || meta.ID == "" {
			continue // StateNone — not shared
		}
		copies, present := store[meta.ID]
		switch {
		case !present:
			out[mm.Path] = StateMissing
		case matchesAny(mm.Raw, copies):
			out[mm.Path] = StateSynced
		default:
			out[mm.Path] = StateDiffers
		}
	}
	return out, nil
}

// matchesAny reports whether local matches any of the store copies (line-ending
// tolerant). A memory's id can appear in the store under more than one scope
// (re-promoting to a different scope leaves the old copy behind), so "in sync"
// means the local content matches at least one of them — not whichever the walk
// happened to index last.
func matchesAny(local string, copies []string) bool {
	for _, c := range copies {
		if sameContent(local, c) {
			return true
		}
	}
	return false
}

// storeIndexByID maps engram.id -> the raw contents of every store memory with
// that id, walking both global/ and projects/** recursively. It parallels
// pull.go's indexByID (single-dir, id->path) but spans the whole store and keeps
// content, so SyncStates can compare without a second read. An id can map to more
// than one copy (the same memory promoted under two scopes), so callers match
// against any of them. Best-effort: a missing or unreadable path is skipped so one
// bad file can't blank the whole feature — badges are decorative, never fatal.
func storeIndexByID(storeDir string) map[string][]string {
	out := map[string][]string{}
	for _, sub := range []string{"global", "projects"} {
		root := filepath.Join(storeDir, sub)
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				if d != nil && d.IsDir() {
					return fs.SkipDir // unreadable dir — skip it, keep scanning the rest
				}
				return nil // missing subtree or unreadable entry — skip, don't abort
			}
			if d.IsDir() || !strings.HasSuffix(d.Name(), ".md") {
				return nil
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				return nil // skip unreadable files rather than fail the whole scan
			}
			if m, ok, _ := memory.ReadEngram(string(raw)); ok && m.ID != "" {
				out[m.ID] = append(out[m.ID], string(raw))
			}
			return nil
		})
	}
	return out
}
