package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ertugrulhaskan/engram/internal/secrets"
	"github.com/ertugrulhaskan/engram/internal/team"
)

// scanFinishedMsg carries the result of scanning a memory before it is promoted.
// path and placement travel with the result so the eventual promote acts on the
// memory that was actually scanned — not on live model state, which a second
// promote started while this scan was in flight could have changed.
type scanFinishedMsg struct {
	path      string
	findings  []secrets.Finding
	placement string // the scope the promote is headed to
	err       error
}

// scanCmd scans the memory at path for secrets off the UI thread. Scope follows
// the configured setting (secrets, or secrets + PII).
func (m Model) scanCmd(path, placement string) tea.Cmd {
	scope := secrets.ScopeSecrets
	if m.scanPII {
		scope = secrets.ScopeSecretsAndPII
	}
	return func() tea.Msg {
		findings, err := team.ScanForSecrets(path, scope)
		return scanFinishedMsg{path: path, findings: findings, placement: placement, err: err}
	}
}

// applyScanResult decides what to do once a pre-promote scan returns: promote when
// clean, block (with or without an override) or warn-and-promote when it isn't.
func (m Model) applyScanResult(msg scanFinishedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		// Don't promote silently if we couldn't even scan.
		return m, m.setDanger("secret scan failed: " + msg.err.Error())
	}
	if len(msg.findings) == 0 {
		return m, m.promoteCmd(msg.path, msg.placement, false)
	}
	if m.scanAction == "warn" {
		return m, tea.Batch(
			m.setDanger(secretSummary(msg.findings)+" — promoting anyway"),
			m.promoteCmd(msg.path, msg.placement, false),
		)
	}
	// block / block-strict — hold the scanned path/placement for the user's call.
	m.secretFindings = msg.findings
	m.secretPath = msg.path
	m.secretPlacement = msg.placement
	m.mode = modeSecretWarn
	return m, nil
}

// updateSecretWarn drives the block modal: n/esc cancels; y overrides and promotes
// anyway — unless the policy is block-strict, where there is no override.
func (m Model) updateSecretWarn(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "ctrl+c", "n":
		m.mode = modeNormal
		m.secretFindings = nil
		return m, m.setCancel("promote cancelled — possible secret")
	case "y":
		if m.scanAction == "block-strict" {
			return m, nil // no override in strict mode
		}
		m.mode = modeNormal
		path, placement := m.secretPath, m.secretPlacement
		m.secretFindings = nil
		return m, m.promoteCmd(path, placement, true)
	}
	return m, nil
}

// secretSummary is the one-line footer form used by the warn policy.
func secretSummary(fs []secrets.Finding) string {
	if len(fs) == 1 {
		return "possible secret (" + fs[0].Rule + ")"
	}
	return fmt.Sprintf("%d possible secrets", len(fs))
}

// secretModal lists the redacted findings that blocked a promote, in the shared
// dialog anatomy (Danger). Each finding names its line and rule with the
// redacted match under it. In block-strict mode there is no override action.
// The spec's caveat is trimmed of its "is recorded" claim — engram keeps no
// audit trail, and the dialog must not pretend otherwise.
func (m Model) secretModal() string {
	t := m.theme()
	cw := m.boxWidth()
	panel := m.panelBg()
	strict := m.scanAction == "block-strict"

	lines := m.dlgHeader(cw, "!", "secret scan blocked this promote", t.Danger)

	const maxShown = 3
	shown, extra := m.secretFindings, 0
	if len(shown) > maxShown {
		extra = len(shown) - maxShown
		shown = shown[:maxShown]
	}
	for _, f := range shown {
		lines = append(lines,
			padBG(onbg(t.Fg, panel).Render(clip(fmt.Sprintf("  line %d · %s", f.Line, f.Rule), cw)), cw, panel),
			padBG(onbg(t.Dim, panel).Render(clip("  "+f.Match, cw)), cw, panel))
	}
	if extra > 0 {
		lines = append(lines, padBG(onbg(t.Dim, panel).Render(fmt.Sprintf("  +%d more", extra)), cw, panel))
	}
	lines = append(lines, padBG("", cw, panel))
	caveat := "The scan is a curated rule set — a guard, not a guarantee. Overriding is a real decision."
	if strict {
		caveat += " This policy is block-strict: remove the secret, then promote."
	}
	lines = append(lines, m.dlgText(cw, caveat, t.Dim)...)
	lines = append(lines, padBG("", cw, panel))

	actions := []dialogAction{{"esc cancel", false}}
	if !strict {
		actions = append(actions, dialogAction{"y override", true})
	}
	bleed := map[int]string{len(lines): t.Bg2}
	lines = append(lines, m.dlgFooter(cw, t.Danger, actions))
	return m.frameLines(lines, cw, t.Danger, bleed)
}
