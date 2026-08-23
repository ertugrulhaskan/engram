package team

import (
	"os"
	"path/filepath"
	"strings"
)

// withdrawnLedger is a file at the store root recording engram ids that have been
// withdrawn — a tombstone list. Withdraw appends an id and commits/pushes it; a
// teammate's Pull reads it and removes the matching local copy, so a withdrawal
// propagates on sync. Re-promoting an id clears its entry, so a re-shared memory
// is not deleted by a stale tombstone. Each line is "<id>\t<slug>"; blank lines
// and "#" comments are ignored.
//
// There is deliberately no prune, and that note belongs here rather than only in
// the spec, because this is the file someone would come to in order to add one. A
// tombstone is safe to drop only after every clone has pulled past it, and a git
// store knows neither how many clones exist nor what they have synced. Drop one
// early and a clone that never saw it never deletes its copy — the memory stays
// scope:team on that machine, still read by Claude there. It does not return to
// the store (Pull never writes to it), so the blast radius is one clone; but that
// is the failure this file exists to prevent.
//
// The cost of keeping them was measured, not assumed. The ledger holds one line per
// distinct currently-withdrawn id, not one per withdrawal — removeWithdrawn clears
// the entry on re-promote — so its ceiling is the number of distinct memories ever
// shared here and left withdrawn, which no loop can run up. readWithdrawn runs once
// per Pull. At 100,000 entries the file is 7.1 MB and the read takes 15 ms, under
// the network round-trip of the git pull that just ran. Keeping a stale tombstone
// costs bytes; dropping a live one costs correctness. SPEC §7 records the
// alternatives weighed.
const withdrawnLedger = ".engram-withdrawn"

// readWithdrawn returns the set of withdrawn ids recorded in the store.
func readWithdrawn(dir string) map[string]bool {
	out := map[string]bool{}
	data, err := os.ReadFile(filepath.Join(dir, withdrawnLedger))
	if err != nil {
		return out
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out[strings.Fields(line)[0]] = true
	}
	return out
}

// addWithdrawn records id (with a human-readable slug) in the ledger unless it is
// already there. It returns the ledger's store-relative path when the file
// changed (so the caller can stage it), or "" when nothing changed.
func addWithdrawn(dir, id, slug string) string {
	if readWithdrawn(dir)[id] {
		return ""
	}
	p := filepath.Join(dir, withdrawnLedger)
	existing, _ := os.ReadFile(p) // absent/unreadable → empty, treated as no prior entries
	var b strings.Builder
	b.Write(existing)
	// A ledger left without a trailing newline (an external edit or a hand-resolved
	// git merge) must not fuse the previous entry with this one — `strings.Fields`
	// would then read only the first id and silently drop this withdrawal.
	if len(existing) > 0 && !strings.HasSuffix(string(existing), "\n") {
		b.WriteByte('\n')
	}
	b.WriteString(id + "\t" + slug + "\n")
	if err := os.WriteFile(p, []byte(b.String()), 0o644); err != nil {
		return ""
	}
	return withdrawnLedger
}

// removeWithdrawn drops id from the ledger (used when an id is re-promoted). It
// returns the ledger's store-relative path when the file changed, or "".
func removeWithdrawn(dir, id string) string {
	p := filepath.Join(dir, withdrawnLedger)
	data, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	var kept []string
	changed := false
	for _, line := range strings.Split(string(data), "\n") {
		t := strings.TrimSpace(line)
		if t != "" && !strings.HasPrefix(t, "#") && strings.Fields(t)[0] == id {
			changed = true
			continue
		}
		kept = append(kept, line)
	}
	if !changed {
		return ""
	}
	if err := os.WriteFile(p, []byte(strings.Join(kept, "\n")), 0o644); err != nil {
		return ""
	}
	return withdrawnLedger
}
