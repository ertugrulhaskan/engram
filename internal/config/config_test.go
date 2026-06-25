package config

import (
	"path/filepath"
	"testing"
)

func TestDir(t *testing.T) {
	base := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", base)

	got, err := Dir()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(base, "engram"); got != want {
		t.Errorf("Dir() = %q, want %q", got, want)
	}
}

func TestRoundTrip(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	// Absent file → zero Config.
	if c := Load(); c.Theme != "" || c.Editor != "" {
		t.Fatalf("missing config should be zero, got %+v", c)
	}

	want := Config{Theme: "Nord", Editor: "code --wait"}
	if err := Save(want); err != nil {
		t.Fatal(err)
	}
	if got := Load(); got != want {
		t.Errorf("round trip: got %+v, want %+v", got, want)
	}
}
