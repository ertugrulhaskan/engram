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

// flaggedMemory is one memory the pre-promote scan objected to, kept with its
// findings and title so the walk can ask about it by name.
type flaggedMemory struct {
	item     batchItem
	findings []secrets.Finding
}

// batchScanFinishedMsg carries the scan for a whole marked set. Every memory is
// scanned before any decision is asked for, so the user is never part-way through
// approving a batch when a later scan turns up something.
type batchScanFinishedMsg struct {
	clean   []batchItem
	flagged []flaggedMemory
	err     error
}

// batchScanCmd scans every memory in the batch off the UI thread. A batch promote
// must not become a way to slip a credential past the guard, so the scan runs per
// memory — the first clean one does not vouch for the rest.
func (m Model) batchScanCmd(items []batchItem) tea.Cmd {
	scope := secrets.ScopeSecrets
	if m.scanPII {
		scope = secrets.ScopeSecretsAndPII
	}
	return func() tea.Msg {
		var res batchScanFinishedMsg
		for _, b := range items {
			findings, err := team.ScanForSecrets(b.path, scope)
			if err != nil {
				// Don't promote anything we couldn't even scan.
				return batchScanFinishedMsg{err: fmt.Errorf("%s: %v", b.title, err)}
			}
			if len(findings) == 0 {
				res.clean = append(res.clean, b)
				continue
			}
			res.flagged = append(res.flagged, flaggedMemory{item: b, findings: findings})
		}
		return res
	}
}

// applyBatchScanResult opens the per-memory walk, or promotes straight away when
// the scan had nothing to say.
func (m Model) applyBatchScanResult(msg batchScanFinishedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		return m, m.setDanger("secret scan failed: " + msg.err.Error())
	}
	if len(msg.flagged) == 0 {
		return m, m.promoteBatchCmd(msg.clean, 0, 0)
	}
	if m.scanAction == "warn" {
		all := append(append([]batchItem{}, msg.clean...), flaggedItems(msg.flagged)...)
		return m, tea.Batch(
			m.setDanger(batchSecretSummary(msg.flagged)+" — promoting anyway"),
			m.promoteBatchCmd(all, 0, len(msg.flagged)),
		)
	}
	// block / block-strict — ask about each flagged memory in turn.
	m.scanAccepted = msg.clean
	m.scanFlagged = msg.flagged
	m.scanIdx = 0
	m.scanSkipped, m.scanOverrode = 0, 0
	m.mode = modeSecretWarn
	m.loadFlagged()
	return m, nil
}

// loadFlagged points the block modal at the flagged memory currently being asked
// about.
func (m *Model) loadFlagged() {
	f := m.scanFlagged[m.scanIdx]
	m.secretFindings = f.findings
	m.secretPath = f.item.path
	m.secretTitle = f.item.title
	m.secretPlacement = f.item.placement
}

// advanceFlagged moves to the next flagged memory, or finishes the walk: the
// accepted memories go as one commit, and a walk that accepted nothing simply
// reports that rather than making an empty commit.
func (m Model) advanceFlagged() (tea.Model, tea.Cmd) {
	m.scanIdx++
	if m.scanIdx < len(m.scanFlagged) {
		m.loadFlagged()
		return m, nil
	}
	accepted, skipped, overrode := m.scanAccepted, m.scanSkipped, m.scanOverrode
	// Summarise before the reset clears scanFlagged. skipped counts *memories*;
	// wording it as a secret count made two memories carrying five findings
	// each report "2 possible secrets". batchSecretSummary already draws that
	// distinction — reuse it rather than keeping a second, wronger phrasing.
	declined := batchSecretSummary(m.scanFlagged)
	m.mode = modeNormal
	m.scanFlagged, m.scanAccepted, m.secretFindings = nil, nil, nil
	m.scanIdx, m.scanSkipped, m.scanOverrode, m.secretTitle = 0, 0, 0, ""
	if len(accepted) == 0 {
		return m, m.setCancel("promote cancelled — " + declined)
	}
	return m, m.promoteBatchCmd(accepted, skipped, overrode)
}

// flaggedItems unwraps the memories from their findings.
func flaggedItems(fs []flaggedMemory) []batchItem {
	out := make([]batchItem, 0, len(fs))
	for _, f := range fs {
		out = append(out, f.item)
	}
	return out
}

// batchSecretSummary is the one-line footer form for a batch under the warn policy.
func batchSecretSummary(fs []flaggedMemory) string {
	n := 0
	for _, f := range fs {
		n += len(f.findings)
	}
	return fmt.Sprintf("%s in %s", pluralLine(n, "1 possible secret", "%d possible secrets"),
		pluralLine(len(fs), "1 memory", "%d memories"))
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
	// Batch walk: n skips just this memory, y takes it, esc abandons the whole
	// batch. Skipping is available even under block-strict, because it overrides
	// nothing — it promotes less, which is what strict wants.
	if len(m.scanFlagged) > 0 {
		switch msg.String() {
		case "esc", "ctrl+c":
			m.mode = modeNormal
			// Same distinction as advanceFlagged's cancel line: this counted
			// memories while its singular form said "possible secret".
			abandoned := batchSecretSummary(m.scanFlagged)
			m.scanFlagged, m.scanAccepted, m.secretFindings = nil, nil, nil
			m.batchItems = nil
			m.scanIdx, m.scanSkipped, m.scanOverrode, m.secretTitle = 0, 0, 0, ""
			return m, m.setCancel("batch cancelled — " + abandoned)
		case "n":
			m.scanSkipped++
			return m.advanceFlagged()
		case "y":
			if m.scanAction == "block-strict" {
				return m, nil // no override in strict mode
			}
			m.scanAccepted = append(m.scanAccepted, m.scanFlagged[m.scanIdx].item)
			m.scanOverrode++
			return m.advanceFlagged()
		}
		return m, nil
	}
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

	batch := len(m.scanFlagged) > 0
	head := "secret scan blocked this promote"
	if batch {
		// Name the memory and where we are in the walk: under a per-memory
		// decision the user has to know which file they are judging.
		head = fmt.Sprintf("secret scan · %s (%d of %d)",
			clip(m.secretTitle, cw-28), m.scanIdx+1, len(m.scanFlagged))
	}
	lines := m.dlgHeader(cw, "!", head, t.Danger)

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
	caveat := "The scan reads key formats, secret-ish names, and generated-looking values — a guard, not a guarantee. Overriding is a real decision."
	if strict {
		caveat += " This policy is block-strict: remove the secret, then promote."
	}
	if batch {
		caveat += " Skipping leaves this memory personal; the rest of the batch still goes as one commit."
	}
	lines = append(lines, m.dlgText(cw, caveat, t.Dim)...)
	lines = append(lines, padBG("", cw, panel))

	actions := []dialogAction{{"esc cancel", false}}
	if batch {
		actions = []dialogAction{{"esc cancel batch", false}, {"n skip this", false}}
	}
	if !strict {
		label := "y override"
		if batch {
			label = "y include"
		}
		actions = append(actions, dialogAction{label, true})
	}
	bleed := map[int]string{len(lines): t.Bg2}
	lines = append(lines, m.dlgFooter(cw, t.Danger, actions))
	return m.frameLines(lines, cw, t.Danger, bleed)
}
