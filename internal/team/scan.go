package team

import (
	"os"

	"github.com/ertugrulhaskan/engram/internal/secrets"
)

// ScanForSecrets reads the memory file at memPath and scans it for credentials
// before it is promoted to the shared store. The TUI calls this first and applies
// the configured policy (block / block-strict / warn / off); Promote itself stays
// a pure push, so the scan lives where the file IO already belongs (not in the UI).
// Promote re-reads the file at push time, so an external edit between the scan and
// the push is not covered — acceptable for a single-user local TUI.
func ScanForSecrets(memPath string, scope secrets.Scope) ([]secrets.Finding, error) {
	raw, err := os.ReadFile(memPath)
	if err != nil {
		return nil, err
	}
	return secrets.Scan(string(raw), scope), nil
}
