package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

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
		return m, m.promoteCmd(msg.path, msg.placement)
	}
	if m.scanAction == "warn" {
		return m, tea.Batch(
			m.setDanger(secretSummary(msg.findings)+" — promoting anyway"),
			m.promoteCmd(msg.path, msg.placement),
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
		return m, m.promoteCmd(path, placement)
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

// secretModal lists the redacted findings that blocked a promote, styled like the
// delete confirmation (danger frame). In block-strict mode it drops the override.
func (m Model) secretModal() string {
	t := m.theme()
	cw := m.boxWidth()
	panel := m.panelBg()
	pst := func(col string) lipgloss.Style { return fg(col).Background(lipgloss.Color(panel)) }
	strict := m.scanAction == "block-strict"

	lines := []string{
		padBG(pst(t.Danger).Bold(true).Render(" Possible secret in this memory"), cw, panel),
		m.ruleLine(cw),
	}

	const maxShown = 4
	shown, extra := m.secretFindings, 0
	if len(shown) > maxShown {
		extra = len(shown) - maxShown
		shown = shown[:maxShown]
	}
	for _, f := range shown {
		row := fmt.Sprintf("  %s · line %d · %s", f.Rule, f.Line, f.Match)
		lines = append(lines, padBG(pst(t.Fg).Render(clip(row, cw)), cw, panel))
	}
	if extra > 0 {
		lines = append(lines, padBG(pst(t.Dim).Render(fmt.Sprintf("  +%d more", extra)), cw, panel))
	}

	var hint string
	if strict {
		hint = pst(t.Dim).Render("  remove it, then promote — ") + pst(t.Fg).Bold(true).Render("esc") + pst(t.Dim).Render(" cancel")
	} else {
		hint = pst(t.Dim).Render("  share anyway? ") + pst(t.Danger).Bold(true).Render("y") + pst(t.Dim).Render(" promote   ") +
			pst(t.Fg).Bold(true).Render("n") + pst(t.Dim).Render(" / ") + pst(t.Fg).Bold(true).Render("esc") + pst(t.Dim).Render(" cancel")
	}
	lines = append(lines, padBG("", cw, panel), padBG(hint, cw, panel))
	return m.frameLines(lines, cw, t.Danger, nil)
}
