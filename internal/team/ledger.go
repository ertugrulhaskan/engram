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
	f, err := os.OpenFile(filepath.Join(dir, withdrawnLedger), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return ""
	}
	defer f.Close()
	if _, err := f.WriteString(id + "\t" + slug + "\n"); err != nil {
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
