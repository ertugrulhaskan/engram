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
	StateNone       SyncState = iota // not shared (personal, or no engram block)
	StateSynced                      // scope=team, shared content identical to the store
	StateIncoming                    // scope=team, local at base but the store advanced — safe to take
	StateLocalAhead                  // scope=team, local advanced but the store is still at base — promote to share
	StateDiverged                    // scope=team, local and store both advanced past the base — conflict
	StateDiffers                     // scope=team, content differs but no anchor to name a direction
	StateMissing                     // scope=team, but no matching id in the store
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
		if err != nil {
			// The engram block failed to parse. If a block is nonetheless present
			// (corrupted by a bad merge or hand-edit), surface it as ● differs so it
			// doesn't silently masquerade as a personal memory; a file with no block
			// at all stays StateNone.
			if memory.EngramPresent(mm.Raw) {
				out[mm.Path] = StateDiffers
			}
			continue
		}
		if !ok || meta.Scope != "team" || meta.ID == "" {
			continue // StateNone — not shared
		}
		copies, present := store[meta.ID]
		if !present {
			out[mm.Path] = StateMissing
			continue
		}
		out[mm.Path] = classify(mm.Raw, meta.SyncedHash, copies)
	}
	return out, nil
}

// classify decides a present team memory's state against the store copies of its
// id. It digests the local copy and every store copy, then defers to relationOf —
// the one place the direction rule lives (shared with Pull's decidePull).
func classify(local, base string, copies []string) SyncState {
	localHash, err := memory.ContentDigest(local)
	if err != nil {
		return StateDiffers // can't hash the local copy — don't guess a direction
	}
	return relationOf(localHash, base, digestSet(copies))
}

// relationOf is the single source of the sync-direction rule, shared by SyncStates
// (which maps it to a badge) and Pull (which maps it to an action). Given the local
// content digest, the base anchor, and the set of store-copy digests, it returns:
// Synced (content matches a store copy), Incoming (local at base, store moved),
// LocalAhead (local moved, store at base), Diverged (both moved), or a
// direction-less Differs when there is no anchor to reason from.
func relationOf(localHash, base string, storeHashes map[string]bool) SyncState {
	if storeHashes[localHash] {
		return StateSynced // same shared content (bookkeeping may differ)
	}
	if base == "" {
		return StateDiffers // no anchor — can't tell which side moved
	}
	localMoved := localHash != base
	// Whether the store moved must not be masked by a stale copy under another scope.
	// An id can live under two scopes (e.g. global/ and projects/<key>/); if the
	// project copy advanced while the global copy is still at base, "any copy equals
	// base" would wrongly read as "store unchanged" and downgrade a real conflict to
	// ↑ ahead. So: the store is at base only when EVERY copy is; if any copy moved
	// past the base, the store has moved.
	someStoreMoved := false
	for h := range storeHashes {
		if h != base {
			someStoreMoved = true
			break
		}
	}
	allStoreAtBase := storeHashes[base] && !someStoreMoved
	switch {
	case !localMoved && someStoreMoved:
		return StateIncoming // I'm at base, the store advanced past it → take it
	case localMoved && allStoreAtBase:
		return StateLocalAhead // I advanced, every store copy is still at base → promote
	case localMoved && someStoreMoved:
		return StateDiverged // both advanced independently → conflict
	default:
		return StateDiffers // local at base yet unmatched — inconsistent; stay conservative
	}
}

// digestSet returns the set of content digests for the given store copies, skipping
// any that fail to hash.
func digestSet(copies []string) map[string]bool {
	out := make(map[string]bool, len(copies))
	for _, c := range copies {
		if h, err := memory.ContentDigest(c); err == nil {
			out[h] = true
		}
	}
	return out
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
			if containsSymlink(storeDir, path) {
				return nil // never read *through* a symlinked store entry (see promote/pull)
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
