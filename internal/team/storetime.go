package team

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ertugrulhaskan/engram/internal/memory"
)

// StoreLastChange returns the commit time of the last change to the store copy
// of the memory with the given engram id — the honest timestamp behind the sync
// strip's "store advanced Xh ago". It locates the store file carrying the id
// (either scope), then asks git for that file's last commit time. Any failure
// (no store, unknown id, git error) is returned as an error so the caller omits
// the stamp — a time is never fabricated. Store file mtimes are useless here:
// they are checkout artifacts of the local clone, not when a teammate changed
// the memory.
func StoreLastChange(id string) (time.Time, error) {
	if id == "" {
		return time.Time{}, errors.New("no engram id")
	}
	if !IsInitialized() {
		return time.Time{}, errors.New("no team store")
	}
	dir, err := Dir()
	if err != nil {
		return time.Time{}, err
	}
	rel, err := storePathByID(dir, id)
	if err != nil {
		return time.Time{}, err
	}
	// "./" prefix: even after "--", git reads a leading ":" as pathspec magic;
	// anchoring the path keeps a strangely-named store file a literal pathspec.
	out, err := runGitCapture(dir, "log", "-1", "--format=%ct", "--", "./"+rel)
	if err != nil || out == "" {
		return time.Time{}, errors.New("git log gave no time for " + rel)
	}
	sec, err := strconv.ParseInt(out, 10, 64)
	if err != nil {
		return time.Time{}, err
	}
	return time.Unix(sec, 0), nil
}

// storePathByID walks the store for the first .md whose engram block carries
// the id and returns its store-relative path. It mirrors storeIndexByID's walk
// (both scopes, symlink-guarded, best-effort) but keeps the path instead of the
// content — that index serves content comparison; git needs the file's path.
func storePathByID(storeDir, id string) (string, error) {
	var found string
	for _, sub := range []string{"global", "projects"} {
		if found != "" {
			break
		}
		root := filepath.Join(storeDir, sub)
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				if d != nil && d.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			if found != "" {
				return fs.SkipAll
			}
			if d.IsDir() || !strings.HasSuffix(d.Name(), ".md") {
				return nil
			}
			if containsSymlink(storeDir, path) {
				return nil // never act through a symlinked store entry (see promote/pull)
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				return nil
			}
			if m, ok, _ := memory.ReadEngram(string(raw)); ok && m.ID == id {
				if rel, err := filepath.Rel(storeDir, path); err == nil {
					found = rel
				}
				return fs.SkipAll
			}
			return nil
		})
	}
	if found == "" {
		return "", errors.New("id not in store")
	}
	return found, nil
}
