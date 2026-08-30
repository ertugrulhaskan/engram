package team

import (
	"strings"
	"testing"
)

// NormalizeAlias lowercases (a case-only difference must not become two store
// buckets) and admits exactly one safe path component.
func TestNormalizeAlias(t *testing.T) {
	good := map[string]string{
		"acme-app":      "acme-app",
		"  Acme-App  ":  "acme-app",
		"app_v2.0":      "app_v2.0",
		"9lives":        "9lives",
		"a":             "a",
		"weird..middle": "weird..middle", // one component; placementPath only rejects a bare ".."
	}
	for in, want := range good {
		got, err := NormalizeAlias(in)
		if err != nil || got != want {
			t.Errorf("NormalizeAlias(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	bad := []string{
		"",
		"   ",
		"acme/app", // a slash would nest a directory
		".hidden",  // leading dot: hidden dir, and ".." itself starts with one
		"..",
		"-dash",     // must start with a letter or digit
		"acme app",  // whitespace inside
		"acme\\app", // backslash is a separator on Windows
		"acme:app",
		strings.Repeat("a", maxAliasLen+1),   // only the length rule can reject this one
		"acme.",                              // NTFS drops a trailing dot, merging it into "acme"
		"con", "nul", "com0", "com1", "lpt9", // Windows device names — no directory can carry them
		"con.txt", // reserved with an extension too
	}
	for _, in := range bad {
		if got, err := NormalizeAlias(in); err == nil {
			t.Errorf("NormalizeAlias(%q) = %q, want an error", in, got)
		}
	}
}

// CleanAliases is what every consumer reads: values normalized, a malformed
// name dropped, and a name two memory dirs both claim dropped from both.
func TestCleanAliases(t *testing.T) {
	got, dropped := CleanAliases(map[string]string{
		"/a/memory": "  Acme-App ",
		"/b/memory": "bad/alias",
		"/c/memory": "shared",
		"/d/memory": "SHARED",
		"/e/memory": "solo",
	})
	want := map[string]string{"/a/memory": "acme-app", "/e/memory": "solo"}
	if len(got) != len(want) {
		t.Fatalf("CleanAliases = %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("CleanAliases[%q] = %q, want %q", k, got[k], v)
		}
	}
	// What was dropped is reported, one line each, deterministically.
	if len(dropped) != 2 || !strings.HasPrefix(dropped[0], "/b/memory: ") || dropped[1] != "shared is claimed by /c/memory and /d/memory" {
		t.Errorf("dropped = %q", dropped)
	}
	if clean, _ := CleanAliases(nil); clean == nil {
		t.Error("CleanAliases(nil) = nil, want an empty map")
	}
}

// SetAlias normalizes, refuses a name another memory dir holds — naming the
// holder and how to free it — and lets a project re-set its own.
func TestSetAlias(t *testing.T) {
	aliases := map[string]string{"/a/memory": "acme-app"}
	if err := SetAlias(aliases, "/b/memory", " ACME-app "); err == nil || !strings.Contains(err.Error(), "already keys /a/memory") || !strings.Contains(err.Error(), "projectAliases") {
		t.Errorf("duplicate: err = %v, want the refusal naming the holder", err)
	}
	if _, stored := aliases["/b/memory"]; stored {
		t.Error("a duplicate alias was stored")
	}
	if err := SetAlias(aliases, "/a/memory", "Acme-App"); err != nil || aliases["/a/memory"] != "acme-app" {
		t.Errorf("re-set by the holder: err=%v map=%v", err, aliases)
	}
	if err := SetAlias(aliases, "/b/memory", "bad/alias"); err == nil {
		t.Error("malformed alias accepted")
	}
	if err := SetAlias(aliases, "/b/memory", "Other"); err != nil || aliases["/b/memory"] != "other" {
		t.Errorf("free name: err=%v map=%v", err, aliases)
	}
}

// The alias namespace is reserved at the key level: a remote whose host is
// literally "alias" would share projects/alias/ with alias-keyed projects.
func TestAliasNamespaceReserved(t *testing.T) {
	for _, raw := range []string{"alias:acme/app", "git@alias:acme/app.git", "ssh://alias/acme/app"} {
		if key, err := NormalizeRemote(raw); err == nil {
			t.Errorf("NormalizeRemote(%q) = %q, want a refusal", raw, key)
		}
	}
	if key, err := NormalizeRemote("git@github.com:acme/alias.git"); err != nil || key != "github.com/acme/alias" {
		t.Errorf("a path named alias is fine: key=%q err=%v", key, err)
	}
}

// An alias key sits under projects/alias/<name>/ and passes the store's
// traversal guard, so Promote can place under it unchanged.
func TestAliasKeyPlacement(t *testing.T) {
	key := AliasKey("acme-app")
	if key != "alias/acme-app" || !IsAliasKey(key) || IsAliasKey("github.com/acme/app") {
		t.Fatalf("AliasKey/IsAliasKey: key=%q", key)
	}
	got, err := placementPath(key, "note.md")
	if err != nil {
		t.Fatalf("placementPath(%q): %v", key, err)
	}
	if want := "projects/alias/acme-app/note.md"; got != want {
		t.Errorf("placementPath = %q, want %q", got, want)
	}
}
