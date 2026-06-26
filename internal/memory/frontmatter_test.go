package memory

import (
	"strings"
	"testing"
)

func TestWriteEngramPreservesClaudeFrontmatter(t *testing.T) {
	raw := "---\nname: my-memory\ndescription: a hook\nmetadata:\n  type: project\n---\n# My memory\n\nbody line\n"
	meta := EngramMeta{ID: "abc", Scope: "team", Project: "github.com/acme/app", Owner: "me@x.com"}

	out, err := WriteEngram(raw, meta)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"name: my-memory", "description: a hook", "type: project"} {
		if !strings.Contains(out, want) {
			t.Errorf("lost Claude frontmatter %q in:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "# My memory\n\nbody line") {
		t.Errorf("body not preserved:\n%s", out)
	}
	got, ok, err := ReadEngram(out)
	if err != nil || !ok {
		t.Fatalf("ReadEngram: ok=%v err=%v", ok, err)
	}
	if got != meta {
		t.Errorf("engram round trip: got %+v want %+v", got, meta)
	}
}

func TestWriteEngramOnPlainMarkdown(t *testing.T) {
	raw := "# Plain memory\n\njust markdown, no frontmatter\n"
	meta := EngramMeta{ID: "id1", Scope: "team", Project: "global"}

	out, err := WriteEngram(raw, meta)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out, "---\n") {
		t.Errorf("expected a frontmatter block, got:\n%s", out)
	}
	if !strings.Contains(out, "# Plain memory\n\njust markdown") {
		t.Errorf("body not preserved:\n%s", out)
	}
	if strings.Contains(out, "name:") || strings.Contains(out, "metadata:") {
		t.Errorf("invented Claude frontmatter:\n%s", out)
	}
	got, ok, _ := ReadEngram(out)
	if !ok || got != meta {
		t.Errorf("round trip: ok=%v got %+v want %+v", ok, got, meta)
	}
}

func TestWriteEngramUpdatesInPlace(t *testing.T) {
	raw := "---\nname: m\nengram:\n  id: keep-me\n  scope: personal\n---\nbody\n"

	pre, ok, _ := ReadEngram(raw)
	if !ok || pre.ID != "keep-me" {
		t.Fatalf("precondition: %+v ok=%v", pre, ok)
	}

	out, err := WriteEngram(raw, EngramMeta{ID: "keep-me", Scope: "team", Project: "global"})
	if err != nil {
		t.Fatal(err)
	}
	got, _, _ := ReadEngram(out)
	if got.Scope != "team" || got.ID != "keep-me" {
		t.Errorf("update: got %+v", got)
	}
	if n := strings.Count(out, "engram:"); n != 1 {
		t.Errorf("expected exactly one engram block, found %d:\n%s", n, out)
	}
	if !strings.Contains(out, "name: m") {
		t.Errorf("lost Claude key 'name':\n%s", out)
	}
}

func TestWriteEngramRejectsNonMapping(t *testing.T) {
	// Frontmatter that is a YAML list (not a mapping) must error rather than be
	// silently dropped — engram never discards Claude's content.
	if _, err := WriteEngram("---\n- a\n- b\n---\nbody\n", EngramMeta{ID: "x"}); err == nil {
		t.Error("expected an error for non-mapping frontmatter")
	}
}

func TestReadEngramAbsent(t *testing.T) {
	if _, ok, err := ReadEngram("---\nname: m\n---\nbody\n"); ok || err != nil {
		t.Errorf("expected no engram block: ok=%v err=%v", ok, err)
	}
	if _, ok, err := ReadEngram("# plain\n\nbody\n"); ok || err != nil {
		t.Errorf("plain markdown: ok=%v err=%v", ok, err)
	}
}

func TestNewID(t *testing.T) {
	id, err := NewID()
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(id, "-")
	if len(parts) != 5 || len(parts[0]) != 8 || len(parts[1]) != 4 ||
		len(parts[2]) != 4 || len(parts[3]) != 4 || len(parts[4]) != 12 {
		t.Errorf("malformed uuid %q", id)
	}
	if parts[2][0] != '4' {
		t.Errorf("expected version-4 uuid, got %q", id)
	}
	if id2, _ := NewID(); id2 == id {
		t.Error("ids are not unique")
	}
}
