package config

import (
	"os"
	"path/filepath"
	"testing"
)

// Update refuses to write over a config that doesn't parse — the user's
// settings would otherwise be replaced with defaults — and creates a missing
// one; every setting the mutation doesn't touch survives the round trip.
func TestUpdate(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	p := filepath.Join(home, "engram", "config.json")

	if err := Update(func(c *Config) error { c.Theme = "crt"; return nil }); err != nil {
		t.Fatalf("Update on an absent file: %v", err)
	}
	if got := Load(); got.Theme != "crt" {
		t.Errorf("after first Update: theme = %q, want crt", got.Theme)
	}
	if err := Update(func(c *Config) error { c.ScanRoots = []string{"/w"}; return nil }); err != nil {
		t.Fatal(err)
	}
	if got := Load(); got.Theme != "crt" || len(got.ScanRoots) != 1 {
		t.Errorf("round trip lost a setting: %+v", got)
	}

	broken := []byte(`{"theme": "crt",}`)
	if err := os.WriteFile(p, broken, 0o644); err != nil {
		t.Fatal(err)
	}
	err := Update(func(c *Config) error { c.Theme = "paperback"; return nil })
	if err == nil {
		t.Fatal("Update over an unparseable file succeeded")
	}
	if b, _ := os.ReadFile(p); string(b) != string(broken) {
		t.Errorf("the unparseable file was rewritten:\n%s", b)
	}
}
