package team

import (
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ertugrulhaskan/engram/internal/memory"
)

// Remote names the connected team store for display: its origin URL with any
// user-info removed (an https remote can carry a token, and this string is
// shown on screen), a plain label when the clone has no readable origin, or ""
// when no store is set up.
func Remote() string {
	if !IsInitialized() {
		return ""
	}
	dir, err := Dir()
	if err != nil {
		return "a team store (location unknown)"
	}
	u, err := runGitCapture(dir, "remote", "get-url", "origin")
	if err != nil || u == "" {
		return "a team store with no origin remote"
	}
	return displayURL(u)
}

// displayURL drops the user-info, query and fragment from a URL that has a
// scheme (https://user:token@host/r.git?private_token=x becomes
// https://host/r.git), where git hosts put credentials; a token someone puts
// in the path itself is not recognised. An scp-style git@host:path has no
// scheme and no password syntax, so it is shown as is.
func displayURL(raw string) string {
	if !strings.Contains(raw, "://") {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "a team store (unreadable remote URL)"
	}
	u.User, u.RawQuery, u.ForceQuery, u.Fragment, u.RawFragment = nil, "", false, "", ""
	return u.String()
}

// ErrNoStore reports a disconnect with no team store set up.
var ErrNoStore = errors.New("no team store is set up")

// ErrStoreBusy reports a disconnect asked for while a promote, withdraw, pull
// or resolve is still working in the store (storeMu).
var ErrStoreBusy = errors.New("another team action is still running; try again when it finishes")

// Disconnect is the inverse of InitTeam: it marks every team-scoped memory in
// memPaths personal again and removes the store clone, so `>init` can connect
// another one (re-pointing is disconnect, then init).
//
// memPaths is every memory the caller knows of, not only the ones it last saw
// as team: each file is read again here, under storeMu, so a memory a promote
// stamped after the caller's snapshot is kept as personal too. A file deleted
// since is skipped, and so is one whose frontmatter fails to parse and has no
// engram line, since it was never shared. One that fails to parse but has an
// engram line may be a shared memory, so it stops the disconnect.
//
// Nothing is deleted from the memories or from the remote (ENGR-20, decided
// 2026-11-08): each team memory keeps its body and its engram.id, so promoting
// it again re-shares it under the same identity, and the remote still holds
// every shared copy. Only engram's own clone goes away, and only once git
// confirms it holds nothing the remote lacks: uncommitted changes or unpushed
// commits refuse the disconnect rather than vanish with the clone.
//
// It reads and checks every memory before writing any, so a memory it can't
// read stops it with nothing changed; the clone is removed last, so a failure
// part way leaves the store in place and running it again finishes the job.
// When Phase 6 brings several stores, this becomes "disconnect this one".
func Disconnect(memPaths []string) (kept int, err error) {
	if !storeMu.TryLock() {
		return 0, ErrStoreBusy
	}
	defer storeMu.Unlock()
	if !IsInitialized() {
		return 0, ErrNoStore
	}
	dir, err := Dir()
	if err != nil {
		return 0, err
	}
	if err := storeHasNothingPending(dir); err != nil {
		return 0, err
	}

	type demotion struct {
		path, stamped string
	}
	var plan []demotion
	seen := make(map[string]bool, len(memPaths))
	for _, p := range memPaths {
		if seen[p] {
			continue // listed twice: demote and count it once
		}
		seen[p] = true
		raw, err := os.ReadFile(p)
		if errors.Is(err, fs.ErrNotExist) {
			continue // deleted since the caller loaded it: nothing to keep
		}
		if err != nil {
			return 0, fmt.Errorf("reading %s: %v", filepath.Base(p), err)
		}
		meta, ok, err := memory.ReadEngram(string(raw))
		if err != nil {
			if !memory.EngramPresent(string(raw)) && !memory.HasEngramLine(string(raw)) {
				continue // no engram key at all: it was never shared
			}
			return 0, fmt.Errorf("%s: %v — fix its frontmatter, then disconnect again", filepath.Base(p), err)
		}
		if !ok || meta.Scope != "team" {
			continue
		}
		stamped, err := memory.WriteEngram(string(raw), memory.EngramMeta{ID: meta.ID, Scope: "personal"})
		if err != nil {
			return 0, fmt.Errorf("%s: %v", filepath.Base(p), err)
		}
		plan = append(plan, demotion{p, stamped})
	}
	for _, d := range plan {
		if err := os.WriteFile(d.path, []byte(d.stamped), 0o644); err != nil {
			return kept, fmt.Errorf("updating %s: %v (run disconnect again to finish)", filepath.Base(d.path), err)
		}
		kept++
	}
	// Move the clone aside in one step, then delete the moved copy. A delete
	// that failed part way through the clone itself could remove .git and keep
	// the rest, and then neither disconnect (no store) nor init (folder not
	// empty) would touch it. After the rename the store is gone either way; a
	// leftover copy blocks nothing.
	aside := fmt.Sprintf("%s.disconnected-%d", dir, time.Now().UnixNano())
	if err := os.Rename(dir, aside); err != nil {
		return kept, fmt.Errorf("removing the team store at %s: %v", dir, err)
	}
	if err := os.RemoveAll(aside); err != nil {
		return kept, fmt.Errorf("disconnected, but the old copy at %s could not be deleted (%v); delete it yourself", aside, err)
	}
	return kept, nil
}

// storeHasNothingPending refuses a store clone holding work its remote lacks:
// uncommitted changes, or commits a failed push left behind. Unpushed commits
// are counted over every local branch against every remote-tracking branch
// rather than @{u}, so a clone whose first push at init failed (it has no
// upstream yet) is still judged, and one with no commit at all passes. The message points at git rather than >pull,
// because a pull with nothing to bring down never reaches the flush that would
// push them.
func storeHasNothingPending(dir string) error {
	status, err := runGitCapture(dir, "status", "--porcelain")
	if err != nil {
		return fmt.Errorf("checking the team store: %v", err)
	}
	if status != "" {
		return fmt.Errorf("the team store has uncommitted changes — commit or discard them with git in %s, then disconnect", dir)
	}
	// Every local branch, plus HEAD when it resolves (a detached HEAD holds
	// commits no branch names; an unborn one holds none and would make
	// rev-list fail), less everything any remote has.
	args := []string{"rev-list", "--count", "--branches"}
	if _, err := runGitCapture(dir, "rev-parse", "--verify", "-q", "HEAD"); err == nil {
		args = append(args, "HEAD")
	}
	ahead, err := runGitCapture(dir, append(args, "--not", "--remotes")...)
	if err != nil {
		return fmt.Errorf("can't tell whether the team store has unpushed commits — check it with git in %s", dir)
	}
	if ahead != "0" {
		return fmt.Errorf("the team store has %s commit(s) the remote hasn't received — push them (git push in %s) or, if you don't need them, delete %s yourself", ahead, dir, dir)
	}
	return nil
}
