package config

import "testing"

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
