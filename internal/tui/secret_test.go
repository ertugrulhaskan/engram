package tui

import (
	"errors"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ertugrulhaskan/engram/internal/secrets"
)

func oneFinding() []secrets.Finding {
	return []secrets.Finding{{Rule: "aws-access-key-id", Line: 1, Match: "AKIA••••"}}
}

func runeKey(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

// A clean scan promotes; a dirty scan under the default block policy opens the
// override modal instead and holds the scanned path + placement + findings.
func TestApplyScanResult_Policies(t *testing.T) {
	clean, cmd := Model{scanAction: "block"}.applyScanResult(scanFinishedMsg{path: "/a.md", placement: "global"})
	if clean.(Model).mode == modeSecretWarn || cmd == nil {
		t.Error("clean scan should promote, not block")
	}

	blocked, cmd := Model{scanAction: "block"}.applyScanResult(scanFinishedMsg{path: "/a.md", findings: oneFinding(), placement: "global"})
	bm := blocked.(Model)
	if bm.mode != modeSecretWarn || cmd != nil {
		t.Errorf("block policy should open the modal and not promote yet (mode=%v)", bm.mode)
	}
	if bm.secretPath != "/a.md" || bm.secretPlacement != "global" || len(bm.secretFindings) != 1 {
		t.Error("block policy must hold the scanned path + placement + findings for a possible override")
	}

	warned, cmd := Model{scanAction: "warn"}.applyScanResult(scanFinishedMsg{path: "/a.md", findings: oneFinding(), placement: "global"})
	if warned.(Model).mode == modeSecretWarn || cmd == nil {
		t.Error("warn policy should promote anyway, not block")
	}
}

// A scan error must fail safe: no override modal, nothing held, and (by the code
// path) no promote — only a danger status.
func TestApplyScanResult_ErrorFailsSafe(t *testing.T) {
	got, _ := Model{scanAction: "block"}.applyScanResult(scanFinishedMsg{err: errors.New("boom"), placement: "global"})
	gm := got.(Model)
	if gm.mode == modeSecretWarn {
		t.Error("a scan error must not open the override modal")
	}
	if len(gm.secretFindings) != 0 {
		t.Error("a scan error must not hold findings")
	}
}

func TestUpdateSecretWarn(t *testing.T) {
	base := func(action string) Model {
		return Model{scanAction: action, mode: modeSecretWarn, secretFindings: oneFinding(), secretPath: "/x.md", secretPlacement: "global"}
	}

	// block: y overrides (closes modal, issues a promote).
	ov, cmd := base("block").updateSecretWarn(runeKey("y"))
	if ov.(Model).mode != modeNormal || cmd == nil {
		t.Error("block: y should close the modal and promote")
	}

	// block-strict: y must NOT override — modal stays, no promote.
	st, cmd := base("block-strict").updateSecretWarn(runeKey("y"))
	if st.(Model).mode != modeSecretWarn || cmd != nil {
		t.Error("block-strict: y must not override")
	}

	// n cancels and clears the held findings.
	cn, _ := base("block").updateSecretWarn(runeKey("n"))
	if cn.(Model).mode != modeNormal || len(cn.(Model).secretFindings) != 0 {
		t.Error("n should cancel and clear findings")
	}
}
