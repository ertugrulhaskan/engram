package team

import (
	"errors"
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
//
// The single-segment case is the one that matters and the one a first pass
// missed: "alias:acme" normalizes to "alias/acme", which is AliasKey("acme")
// byte for byte, so two unrelated projects land in one bucket and applyPull's
// byKey cross-places their memories. Accepting the overlap was tried and
// reverted — the argument for it ("IsAliasKey only decides a caption") was
// reached by checking multi-segment paths only, where the remote does land in a
// subdirectory the alias bucket never reads. A namespace claim has to be checked
// at its shortest form, where the prefix *is* the whole key.
func TestAliasNamespaceReserved(t *testing.T) {
	// The trailing-dot and trailing-space spellings are included because NTFS
	// strips both: distinct directories here, one directory once a teammate
	// checks the store out on Windows.
	for _, raw := range []string{
		"alias:acme/app", "git@alias:acme/app.git", "ssh://alias/acme/app",
		"alias:acme", "ssh://alias/acme", "ALIAS:acme", "alias.:acme", "ssh://alias./acme",
	} {
		key, err := NormalizeRemote(raw)
		if err == nil {
			t.Errorf("NormalizeRemote(%q) = %q, want a refusal", raw, key)
			continue
		}
		// Its own error, so the UI can say git answered fine and engram declined.
		if !errors.Is(err, ErrReservedHost) {
			t.Errorf("NormalizeRemote(%q) err = %v, want ErrReservedHost", raw, err)
		}
	}
	// The exact collision this guards, stated so the reason survives a refactor.
	if AliasKey("acme") != "alias/acme" {
		t.Fatalf("AliasKey(\"acme\") = %q — the collision this test guards has moved", AliasKey("acme"))
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

// The NTFS collapse the reserved host guards against reaches the key through
// its path too: the key becomes a directory path in the store, so a segment
// with a trailing dot or space is a separate bucket here and the same bucket
// once a teammate checks the store out on Windows — and applyPull's byKey would
// then cross-place two unrelated projects' memories, exactly as it would for a
// colliding host.
func TestRemotePathSegmentsSurviveNTFS(t *testing.T) {
	for _, raw := range []string{
		"git@github.com:acme/app..git", "git@github.com:acme/app.", "https://github.com/acme/app.",
		"git@github.com:acme./app", "https://github.com/ACME./APP.",
	} {
		key, err := NormalizeRemote(raw)
		if err != nil {
			t.Errorf("NormalizeRemote(%q): %v", raw, err)
			continue
		}
		if key != "github.com/acme/app" {
			t.Errorf("NormalizeRemote(%q) = %q, want github.com/acme/app", raw, key)
		}
	}
	// A device name in a *path* is deliberately kept: unlike an alias, a remote
	// is not the user's to rename, so refusing it would strand a real repo over
	// a checkout platform its owner may never use.
	if key, err := NormalizeRemote("git@github.com:acme/con.git"); err != nil || key != "github.com/acme/con" {
		t.Errorf("a device-named path segment is kept: key=%q err=%v", key, err)
	}
	// A dot inside a segment is ordinary and must survive.
	if key, err := NormalizeRemote("git@github.com:acme/app.v2.git"); err != nil || key != "github.com/acme/app.v2" {
		t.Errorf("an interior dot must survive: key=%q err=%v", key, err)
	}
}
