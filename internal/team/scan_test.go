package team

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ertugrulhaskan/engram/internal/secrets"
)

func TestScanForSecrets(t *testing.T) {
	dir := t.TempDir()

	dirty := filepath.Join(dir, "dirty.md")
	if err := os.WriteFile(dirty, []byte("# Notes\n\naws key = AKIAIOSFODNN7EXAMPLE\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fs, err := ScanForSecrets(dirty, secrets.ScopeSecrets)
	if err != nil {
		t.Fatalf("ScanForSecrets: %v", err)
	}
	if len(fs) == 0 {
		t.Error("expected a finding for a memory containing an AWS key")
	}

	clean := filepath.Join(dir, "clean.md")
	if err := os.WriteFile(clean, []byte("# Notes\n\nUse pnpm, not npm.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if fs, _ := ScanForSecrets(clean, secrets.ScopeSecrets); len(fs) != 0 {
		t.Errorf("clean memory should have no findings, got %+v", fs)
	}

	if _, err := ScanForSecrets(filepath.Join(dir, "nope.md"), secrets.ScopeSecrets); err == nil {
		t.Error("expected an error for a missing file")
	}
}
