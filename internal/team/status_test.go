package team

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ertugrulhaskan/engram/internal/memory"
)

// teamMem builds a raw team-scoped memory file body with the given id.
func teamMem(name, id, body string) string {
	return "---\nname: " + name + "\nengram:\n    id: " + id +
		"\n    scope: team\n    project: global\n---\n# " + name + "\n\n" + body + "\n"
}

func TestSyncStates_NotInitialized(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir()) // no team/.git under here
	if IsInitialized() {
		t.Fatal("expected an uninitialized store for a fresh XDG dir")
	}
	got, err := SyncStates([]memory.Memory{{Path: "/x/a.md", Raw: teamMem("a", "ID-A", "x")}})
	if err != nil {
		t.Fatalf("SyncStates: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("want empty map when no store, got %v", got)
	}
}

func TestSyncStates(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	store := filepath.Join(xdg, "engram", "team")

	// Mark the store initialized (IsInitialized only stats <store>/.git).
	mustMkdir(t, filepath.Join(store, ".git"))

	// Seed the store: one global memory, one project-scoped memory.
	syncedRaw := teamMem("synced", "ID-SYNCED", "same on both sides")
	writeStore(t, store, "global/synced.md", syncedRaw)
	writeStore(t, store, "projects/github.com/acme/app/differ.md",
		teamMem("differ", "ID-DIFFER", "the TEAM version"))

	mems := []memory.Memory{
		// personal: no engram block → no entry (StateNone).
		{Path: "/local/personal.md", Raw: "---\nname: personal\n---\n# Personal\n\nbody\n"},
		// team, byte-identical to the store copy → Synced.
		{Path: "/local/synced.md", Raw: syncedRaw},
		// team, same id but different content → Differs.
		{Path: "/local/differ.md", Raw: teamMem("differ", "ID-DIFFER", "my LOCAL edit")},
		// team, id not in the store → Missing.
		{Path: "/local/missing.md", Raw: teamMem("missing", "ID-MISSING", "orphan")},
		// team, CRLF line endings vs the store's LF → still Synced (tolerant compare).
		{Path: "/local/crlf.md", Raw: crlf(syncedRaw)},
	}

	got, err := SyncStates(mems)
	if err != nil {
		t.Fatalf("SyncStates: %v", err)
	}

	want := map[string]SyncState{
		"/local/synced.md":  StateSynced,
		"/local/differ.md":  StateDiffers,
		"/local/missing.md": StateMissing,
		"/local/crlf.md":    StateSynced,
	}
	for path, ws := range want {
		if got[path] != ws {
			t.Errorf("%s = %v, want %v", path, got[path], ws)
		}
	}
	if _, ok := got["/local/personal.md"]; ok {
		t.Errorf("personal memory should have no entry, got %v", got["/local/personal.md"])
	}
	if len(got) != len(want) {
		t.Errorf("got %d entries, want %d: %v", len(got), len(want), got)
	}
}

func TestSyncStatesDirection(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	store := filepath.Join(xdg, "engram", "team")
	mustMkdir(t, filepath.Join(store, ".git"))

	plain := func(name, body string) string {
		return "---\nname: " + name + "\n---\n# " + name + "\n\n" + body + "\n"
	}
	digest := func(name, body string) string {
		d, err := memory.ContentDigest(plain(name, body))
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	anchored := func(name, id, body, anchor string) string {
		out, err := memory.WriteEngram(plain(name, body), memory.EngramMeta{
			ID: id, Scope: "team", Project: "global", SyncedHash: anchor,
		})
		if err != nil {
			t.Fatal(err)
		}
		return out
	}

	// INCOMING: local sits at the base; the store advanced past it.
	baseIn := digest("in", "base body")
	writeStore(t, store, "global/in.md", anchored("in", "IN", "advanced body", baseIn))
	// LOCAL-AHEAD: local advanced; the store is still at the base.
	baseAhead := digest("ahead", "base body")
	writeStore(t, store, "global/ahead.md", anchored("ahead", "AHEAD", "base body", baseAhead))
	// DIVERGED: local and store advanced differently from the same base.
	baseDiv := digest("div", "base body")
	writeStore(t, store, "global/div.md", anchored("div", "DIV", "store edit", baseDiv))

	mems := []memory.Memory{
		{Path: "/l/in.md", Raw: anchored("in", "IN", "base body", baseIn)},
		{Path: "/l/ahead.md", Raw: anchored("ahead", "AHEAD", "local edit", baseAhead)},
		{Path: "/l/div.md", Raw: anchored("div", "DIV", "local edit", baseDiv)},
	}
	got, err := SyncStates(mems)
	if err != nil {
		t.Fatalf("SyncStates: %v", err)
	}
	want := map[string]SyncState{
		"/l/in.md":    StateIncoming,
		"/l/ahead.md": StateLocalAhead,
		"/l/div.md":   StateDiverged,
	}
	for p, ws := range want {
		if got[p] != ws {
			t.Errorf("%s = %v, want %v", p, got[p], ws)
		}
	}
}

func TestSyncStates_CrossScopeDuplicateID(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	store := filepath.Join(xdg, "engram", "team")
	mustMkdir(t, filepath.Join(store, ".git"))

	// The same id lives under two scopes with DIFFERENT content — re-promoting a
	// memory to another scope leaves the old copy behind. The local file matches
	// the global copy, so it must read as synced regardless of walk order.
	globalRaw := teamMem("dup", "DUP", "the current global content")
	writeStore(t, store, "global/dup.md", globalRaw)
	writeStore(t, store, "projects/github.com/acme/app/dup.md", teamMem("dup", "DUP", "a stale project copy"))

	got, err := SyncStates([]memory.Memory{{Path: "/local/dup.md", Raw: globalRaw}})
	if err != nil {
		t.Fatalf("SyncStates: %v", err)
	}
	if got["/local/dup.md"] != StateSynced {
		t.Errorf("duplicate id across scopes: got %v, want StateSynced (must match ANY store copy)", got["/local/dup.md"])
	}
}

func crlf(s string) string {
	out := make([]byte, 0, len(s)+8)
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, '\r')
		}
		out = append(out, s[i])
	}
	return string(out)
}

func mustMkdir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
}

func writeStore(t *testing.T, store, rel, content string) {
	t.Helper()
	p := filepath.Join(store, filepath.FromSlash(rel))
	mustMkdir(t, filepath.Dir(p))
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
