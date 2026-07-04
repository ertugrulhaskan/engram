package team

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/ertugrulhaskan/engram/internal/memory"
)

// Conflict markers, git-style. The "yours"/"team" lines are distinctive enough
// that finding them in a saved file means the user hasn't finished resolving.
const (
	conflictOurs = "<<<<<<< yours (local)"
	conflictMid  = "======="
	conflictEnd  = ">>>>>>> team"
)

// storeCopyRaw returns the raw contents of the store copy the local memory tracks,
// and whether one was found. When an id exists under more than one scope (e.g. a
// stale copy left by a re-promote to a different scope), it prefers the copy whose
// project matches the local memory's, so the resolve reconciles against the right
// one rather than whichever the walk indexed first.
func storeCopyRaw(memPath string) (string, bool, error) {
	dir, err := Dir()
	if err != nil {
		return "", false, err
	}
	raw, err := os.ReadFile(memPath)
	if err != nil {
		return "", false, err
	}
	meta, ok, _ := memory.ReadEngram(string(raw))
	if !ok || meta.ID == "" {
		return "", false, nil
	}
	copies := storeIndexByID(dir)[meta.ID]
	if len(copies) == 0 {
		return "", false, nil
	}
	for _, c := range copies {
		if cm, ok, _ := memory.ReadEngram(c); ok && cm.Project == meta.Project {
			return c, true, nil // the copy this memory actually tracks
		}
	}
	return copies[0], true, nil // fall back to any copy of the id
}

// BeginConflictResolve writes a git-style merge of a memory's local and team-store
// bodies to a temp file for the user to reconcile in $EDITOR, and returns its path.
// It errors when the store isn't initialized or has no copy to resolve against
// (e.g. the memory was withdrawn upstream). The caller opens the temp file, then
// hands it back to FinishConflictResolve.
func BeginConflictResolve(memPath string) (string, error) {
	if !IsInitialized() {
		return "", errors.New("no team store — run `engram init-team <git-url>` first")
	}
	localRaw, err := os.ReadFile(memPath)
	if err != nil {
		return "", err
	}
	storeRaw, ok, err := storeCopyRaw(memPath)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", errors.New("no team-store copy to resolve against")
	}
	text, err := conflictText(string(localRaw), storeRaw)
	if err != nil {
		return "", err
	}
	f, err := os.CreateTemp("", "engram-resolve-*.md")
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := f.WriteString(text); err != nil {
		os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}

// conflictText brackets each side's SHARED CONTENT — Claude's frontmatter and body,
// with engram's own block removed — so a divergence in a frontmatter field (not just
// the body) is visible and reconcilable. engram's block is reattached on apply, so
// the user never edits bookkeeping.
func conflictText(localRaw, storeRaw string) (string, error) {
	ls, err := memory.ShareContent(localRaw)
	if err != nil {
		return "", err
	}
	ss, err := memory.ShareContent(storeRaw)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString(conflictOurs + "\n")
	b.WriteString(ensureTrailingNL(ls))
	b.WriteString(conflictMid + "\n")
	b.WriteString(ensureTrailingNL(ss))
	b.WriteString(conflictEnd + "\n")
	return b.String(), nil
}

// FinishConflictResolve applies the user's edited temp file and always removes it.
// An empty file, or one that still contains any conflict-marker line, aborts with
// the memory untouched (resolved=false). Otherwise the edited text — Claude's
// frontmatter and body — becomes the memory's shared content, engram's block is
// reattached, and the anchor is re-based on the store version, so the result reads
// as synced (took theirs) or local-ahead (kept a merge), never a stuck conflict.
func FinishConflictResolve(memPath, tmpPath string) (resolved bool, err error) {
	defer os.Remove(tmpPath)
	edited, err := os.ReadFile(tmpPath)
	if err != nil {
		return false, err
	}
	text := string(edited)
	if strings.TrimSpace(text) == "" {
		return false, nil // aborted: emptied file
	}
	if hasMarkerLine(text) {
		return false, errors.New("conflict markers still present — nothing changed")
	}
	storeRaw, ok, err := storeCopyRaw(memPath)
	if err != nil || !ok {
		return false, errors.New("the team-store copy is gone — cannot complete the resolve")
	}
	localRaw, err := os.ReadFile(memPath)
	if err != nil {
		return false, err
	}
	anchor, err := memory.ContentDigest(storeRaw) // base = the version we reconciled against
	if err != nil {
		return false, fmt.Errorf("hashing the store copy: %v", err)
	}
	meta, _, _ := memory.ReadEngram(string(localRaw))
	meta.SyncedHash = anchor
	stamped, err := memory.WriteEngram(text, meta) // text already holds the resolved frontmatter + body
	if err != nil {
		return false, err
	}
	if err := os.WriteFile(memPath, []byte(stamped), 0o644); err != nil {
		return false, err
	}
	return true, nil
}

// hasMarkerLine reports whether any line of text is exactly a conflict marker
// (trimmed of a trailing CR). Whole-line matching keeps prose like "<<<" inline
// from tripping it; the "=======" check can graze a setext-heading underline, which
// is a safe false-positive — it blocks the save rather than silently persisting a
// half-merged file.
func hasMarkerLine(text string) bool {
	for _, ln := range strings.Split(text, "\n") {
		switch strings.TrimRight(ln, "\r") {
		case conflictOurs, conflictMid, conflictEnd:
			return true
		}
	}
	return false
}

// AbortConflictResolve discards a resolve session's temp file (used when the editor
// exits with an error, so the TUI needn't touch the filesystem itself).
func AbortConflictResolve(tmpPath string) { _ = os.Remove(tmpPath) }

// ensureTrailingNL guarantees a section ends with a newline so the following
// conflict marker starts on its own line.
func ensureTrailingNL(s string) string {
	if s == "" || strings.HasSuffix(s, "\n") {
		return s
	}
	return s + "\n"
}
