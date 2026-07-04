package memory

import "testing"

func TestContentDigestIgnoresEngramBlock(t *testing.T) {
	body := "---\nname: note\ndescription: a hook\n---\n# Note\n\nshared body\n"
	withEngram, err := WriteEngram(body, EngramMeta{
		ID: "abc", Scope: "team", Project: "github.com/acme/app",
		Owner: "me@x.com", SyncedHash: "deadbeef",
	})
	if err != nil {
		t.Fatal(err)
	}
	other, err := WriteEngram(body, EngramMeta{
		ID: "xyz", Scope: "personal", Owner: "someone@else.com", SyncedHash: "99887766",
	})
	if err != nil {
		t.Fatal(err)
	}

	d0, _ := ContentDigest(body)       // no engram block at all
	d1, _ := ContentDigest(withEngram) // one set of engram metadata
	d2, _ := ContentDigest(other)      // different engram metadata
	if d0 == "" {
		t.Fatal("digest should be non-empty")
	}
	if d0 != d1 || d1 != d2 {
		t.Errorf("engram metadata leaked into the digest: %q / %q / %q", d0, d1, d2)
	}
}

func TestContentDigestTracksSharedContent(t *testing.T) {
	base := "---\nname: note\n---\n# Note\n\nline one\n"
	edited := "---\nname: note\n---\n# Note\n\nline one changed\n"
	renamed := "---\nname: renamed\n---\n# Note\n\nline one\n" // Claude frontmatter is shared content

	d, _ := ContentDigest(base)
	if de, _ := ContentDigest(edited); d == de {
		t.Error("a body change must change the digest")
	}
	if dr, _ := ContentDigest(renamed); d == dr {
		t.Error("a Claude-frontmatter change must change the digest")
	}
}

func TestContentDigestLineEndingTolerant(t *testing.T) {
	lf := "---\nname: note\n---\n# Note\n\nbody\n"
	crlf := "---\r\nname: note\r\n---\r\n# Note\r\n\r\nbody\r\n"
	if a, _ := ContentDigest(lf); a != mustDigest(t, crlf) {
		t.Error("CRLF and LF of the same content must digest equal")
	}
}

func mustDigest(t *testing.T, raw string) string {
	t.Helper()
	d, err := ContentDigest(raw)
	if err != nil {
		t.Fatal(err)
	}
	return d
}
