package team

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
	if want := filepath.Join(base, "engram", "team"); got != want {
		t.Errorf("Dir() = %q, want %q", got, want)
	}
}
