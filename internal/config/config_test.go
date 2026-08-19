package config

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestScanDefaults(t *testing.T) {
	var c Config // zero value = keys absent
	if got := c.ScanAction(); got != "block" {
		t.Errorf("default ScanAction = %q, want block", got)
	}
	if c.ScanPII() {
		t.Error("default ScanPII should be false (secrets only)")
	}
	if got := (Config{SecretScanAction: "bogus"}).ScanAction(); got != "block" {
		t.Errorf("unrecognized action should fall back to block, got %q", got)
	}
	for _, a := range []string{"block-strict", "warn", "off"} {
		if got := (Config{SecretScanAction: a}).ScanAction(); got != a {
			t.Errorf("ScanAction(%q) = %q", a, got)
		}
	}
	if !(Config{SecretScanScope: "secrets+pii"}).ScanPII() {
		t.Error("secrets+pii should enable PII scanning")
	}
}

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

	// ScanRoots included: it is a slice, so Config is no longer comparable with
	// != — reflect.DeepEqual is now the round-trip check, and it also proves the
	// slice survives the JSON round trip rather than coming back nil.
	want := Config{Theme: "crt", Editor: "code --wait", ScanRoots: []string{"~/code", "/srv/work"}}
	if err := Save(want); err != nil {
		t.Fatal(err)
	}
	if got := Load(); !reflect.DeepEqual(got, want) {
		t.Errorf("round trip: got %+v, want %+v", got, want)
	}

	// An omitted scanRoots must load as nil, not an empty non-nil slice, so a
	// config written before this field existed still round-trips unchanged.
	bare := Config{Theme: "midnight"}
	if err := Save(bare); err != nil {
		t.Fatal(err)
	}
	if got := Load(); got.ScanRoots != nil {
		t.Errorf("absent scanRoots loaded as %#v, want nil", got.ScanRoots)
	}
}
