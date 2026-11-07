package config

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// Update must keep keys this version doesn't know — a setting from a newer
// engram, or one added by hand. It round-tripped through the typed struct, so
// a theme keypress silently dropped them.
func TestUpdateKeepsUnknownKeys(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	p, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dirOf(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(`{"theme":"crt","editor":"vim","futureKey":{"a":1},"note":"hand-added"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Update(func(c *Config) error { c.Theme = "midnight"; c.Editor = ""; return nil }); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(p)
	var got map[string]json.RawMessage
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("saved config doesn't parse: %v\n%s", err, raw)
	}
	var future map[string]int
	var note string
	if json.Unmarshal(got["futureKey"], &future) != nil || future["a"] != 1 ||
		json.Unmarshal(got["note"], &note) != nil || note != "hand-added" {
		t.Errorf("unknown keys lost:\n%s", raw)
	}
	if string(got["theme"]) != `"midnight"` {
		t.Errorf("the mutation itself was not saved:\n%s", raw)
	}
	if _, still := got["editor"]; still {
		t.Errorf("a known key the mutation cleared came back from the old file:\n%s", raw)
	}
}

func dirOf(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' || p[i] == os.PathSeparator {
			return p[:i]
		}
	}
	return "."
}

// Save keeps unknown keys too, so no exported write path drops them.
func TestSaveKeepsUnknownKeys(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	p, _ := Path()
	_ = os.MkdirAll(dirOf(p), 0o755)
	if err := os.WriteFile(p, []byte(`{"theme":"crt","futureKey":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := Read()
	if err != nil {
		t.Fatal(err)
	}
	c.Theme = "paperback"
	if err := Save(c); err != nil {
		t.Fatal(err)
	}
	var got map[string]json.RawMessage
	raw, _ := os.ReadFile(p)
	if json.Unmarshal(raw, &got) != nil || string(got["futureKey"]) != "true" {
		t.Errorf("Save dropped an unknown key:\n%s", raw)
	}
}

// A known key in another case ("Theme") is decoded into Config by encoding/json,
// so it must not also be kept as unknown and written back beside "theme".
func TestKnownKeyInOtherCaseIsNotKeptTwice(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	p, _ := Path()
	_ = os.MkdirAll(dirOf(p), 0o755)
	if err := os.WriteFile(p, []byte(`{"Theme":"paperback"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Update(func(c *Config) error { c.Theme = "crt"; return nil }); err != nil {
		t.Fatal(err)
	}
	var got map[string]json.RawMessage
	raw, _ := os.ReadFile(p)
	_ = json.Unmarshal(raw, &got)
	if _, stale := got["Theme"]; stale {
		t.Errorf("the differently-cased key was written back:\n%s", raw)
	}
}

// Unknown keys are appended; the known settings keep their usual order.
func TestUnknownKeysDoNotReorderTheFile(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	p, _ := Path()
	_ = os.MkdirAll(dirOf(p), 0o755)
	if err := os.WriteFile(p, []byte(`{"theme":"crt","editor":"vim","aaa":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Update(func(c *Config) error { c.Theme = "paperback"; return nil }); err != nil {
		t.Fatal(err)
	}
	raw := string(must(os.ReadFile(p)))
	if !(strings.Index(raw, `"theme"`) < strings.Index(raw, `"editor"`) && strings.Index(raw, `"editor"`) < strings.Index(raw, `"aaa"`)) {
		t.Errorf("want theme, editor, then the unknown key appended:\n%s", raw)
	}
	var v map[string]any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		t.Errorf("not valid JSON: %v\n%s", err, raw)
	}
}

func must(b []byte, err error) []byte {
	if err != nil {
		panic(err)
	}
	return b
}
