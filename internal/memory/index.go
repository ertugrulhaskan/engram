package memory

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// IndexFile is the per-folder human index Claude Code loads each session.
const IndexFile = "MEMORY.md"

const indexHeader = "# Memory index"

// indexLineFor builds the canonical MEMORY.md bullet for one memory file,
// deriving the title and hook from the file itself (the source of truth).
func indexLineFor(memDir, file string) (string, error) {
	m, err := parseFile(filepath.Join(memDir, file), nil)
	if err != nil {
		return "", err
	}
	line := fmt.Sprintf("- [%s](%s)", m.Title, file)
	if m.Description != "" {
		line += " — " + m.Description
	}
	return line, nil
}

// lineFile returns the memory file a MEMORY.md bullet line names, or "" when
// the line is not a bullet or its link names no memory in this folder (see
// indexTarget).
func lineFile(line string) string {
	if _, target, _, ok := parseIndexLine(line); ok {
		return indexTarget(target)
	}
	return ""
}

// UpsertIndex adds or refreshes file's bullet in memDir/MEMORY.md. An existing
// entry is replaced in place (preserving its position and the rest of the file);
// a new entry is appended after the last bullet. The index is created with a
// header if it doesn't exist.
func UpsertIndex(memDir, file string) error {
	newLine, err := indexLineFor(memDir, file)
	if err != nil {
		return err
	}
	path := filepath.Join(memDir, IndexFile)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return writeIndexFile(path, []byte(indexHeader+"\n\n"+newLine+"\n"))
		}
		return err
	}

	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	lastBullet := -1
	for i, ln := range lines {
		if f := lineFile(ln); f != "" {
			lastBullet = i
			if f == file {
				lines[i] = newLine // replace in place
				return writeLines(path, lines)
			}
		}
	}
	// Not present — insert after the last bullet, else append at the end.
	if lastBullet >= 0 {
		out := make([]string, 0, len(lines)+1)
		out = append(out, lines[:lastBullet+1]...)
		out = append(out, newLine)
		out = append(out, lines[lastBullet+1:]...)
		lines = out
	} else {
		lines = append(lines, newLine)
	}
	return writeLines(path, lines)
}

// RemoveIndex drops file's bullet from memDir/MEMORY.md, leaving the rest
// untouched. It's a no-op if the index doesn't exist.
func RemoveIndex(memDir, file string) error {
	path := filepath.Join(memDir, IndexFile)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	out := lines[:0]
	for _, ln := range lines {
		if lineFile(ln) == file {
			continue
		}
		out = append(out, ln)
	}
	return writeLines(path, out)
}

// UpsertIndexForPath / RemoveIndexForPath are convenience wrappers keyed by a
// memory's full path, so callers needn't split dir and filename themselves.
func UpsertIndexForPath(path string) error {
	return UpsertIndex(filepath.Dir(path), filepath.Base(path))
}

func RemoveIndexForPath(path string) error {
	return RemoveIndex(filepath.Dir(path), filepath.Base(path))
}

// IndexDrift reports the gap between MEMORY.md and the memory files on disk:
// `unindexed` are files with no bullet, `dangling` are bullets whose file is
// gone. Both empty means the index is in sync.
func IndexDrift(memDir string) (unindexed, dangling []string, err error) {
	indexed := map[string]bool{}
	if data, e := os.ReadFile(filepath.Join(memDir, IndexFile)); e == nil {
		for _, ln := range strings.Split(string(data), "\n") {
			if f := lineFile(ln); f != "" {
				indexed[f] = true
			}
		}
	} else if !os.IsNotExist(e) {
		return nil, nil, e
	}

	entries, err := os.ReadDir(memDir)
	if err != nil {
		return nil, nil, err
	}
	onDisk := map[string]bool{}
	for _, f := range entries {
		if f.IsDir() || f.Name() == IndexFile || !strings.HasSuffix(f.Name(), ".md") {
			continue
		}
		onDisk[f.Name()] = true
		if !indexed[f.Name()] {
			unindexed = append(unindexed, f.Name())
		}
	}
	for name := range indexed {
		if !onDisk[name] {
			dangling = append(dangling, name)
		}
	}
	sort.Strings(unindexed)
	sort.Strings(dangling)
	return unindexed, dangling, nil
}

// ReconcileIndex brings MEMORY.md back in sync with the files: it drops dangling
// bullets and appends bullets for unindexed files, preserving the header and the
// existing curated order. It does not rewrite already-correct entries. Returns
// the counts added and removed.
func ReconcileIndex(memDir string) (added, removed int, err error) {
	unindexed, dangling, err := IndexDrift(memDir)
	if err != nil {
		return 0, 0, err
	}
	for _, f := range dangling {
		if err := RemoveIndex(memDir, f); err != nil {
			return 0, 0, err
		}
	}
	for _, f := range unindexed {
		if err := UpsertIndex(memDir, f); err != nil {
			return 0, 0, err
		}
	}
	return len(unindexed), len(dangling), nil
}

func writeLines(path string, lines []string) error {
	return writeIndexFile(path, []byte(strings.Join(lines, "\n")+"\n"))
}

// writeIndexFile replaces MEMORY.md atomically: a temp file beside it, then a
// rename. os.WriteFile truncates before it writes, so a crash, a full disk or a
// kill in between left Claude Code a truncated or empty index — the file it
// loads every session. A symlinked MEMORY.md is followed first, so the rename
// replaces the target and the link survives, and the file's mode is kept. (The
// read-modify-write is still not a lock: an edit Claude makes between engram's
// read and its rename is lost, as before — but never half-written.)
func writeIndexFile(path string, data []byte) error {
	target := path
	if fi, err := os.Lstat(path); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		if r, err := filepath.EvalSymlinks(path); err == nil {
			target = r
		} else if l, err := os.Readlink(path); err == nil { // dangling: write where it points
			if !filepath.IsAbs(l) {
				l = filepath.Join(filepath.Dir(path), l)
			}
			target = l
		}
	}
	mode := os.FileMode(0o644)
	if fi, err := os.Stat(target); err == nil {
		mode = fi.Mode().Perm()
		// A rename needs only the directory, so it would silently replace a
		// MEMORY.md the user made read-only (chmod 444, a read-only dotfiles
		// deploy). Ask the file itself for write access first — no truncate —
		// and refuse as os.WriteFile always did. Same rule as config.openTarget.
		w, err := os.OpenFile(target, os.O_WRONLY, 0)
		if err != nil {
			return err
		}
		w.Close()
		// A rename gives the name a new inode, so every other hard link to
		// MEMORY.md would keep the old content. Write such a file in place.
		if hardLinked(fi) {
			return os.WriteFile(target, data, mode)
		}
	}
	f, err := os.CreateTemp(filepath.Dir(target), ".MEMORY-*.tmp")
	if errors.Is(err, fs.ErrPermission) {
		// A writable MEMORY.md in a directory engram may not create files in
		// (a symlink into a read-only dotfiles checkout): the rename can't
		// happen, so write in place as os.WriteFile always did, rather than
		// fail a write that used to work.
		return os.WriteFile(target, data, mode)
	}
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp) // a no-op once the rename has consumed it
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp, mode); err != nil {
		return err
	}
	return os.Rename(tmp, target)
}
