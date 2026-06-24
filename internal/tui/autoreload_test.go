package tui

import (
	"errors"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ertugrulhaskan/engram/internal/config"
	"github.com/ertugrulhaskan/engram/internal/memory"
)

func ready(t *testing.T) Model {
	t.Helper()
	m, _ := New(sampleMemories(), samplePlans(), sampleDocs(), config.Config{}).Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	return m.(Model)
}

// First poll adopts the signature as the baseline without reloading; an
// identical signature afterwards leaves it untouched.
func TestPollBaselineAdoption(t *testing.T) {
	m := ready(t)
	m2, _ := m.Update(pollResultMsg{sig: "SIG1"})
	if got := m2.(Model).fsSig; got != "SIG1" {
		t.Fatalf("baseline fsSig = %q, want SIG1", got)
	}
	m3, _ := m2.(Model).Update(pollResultMsg{sig: "SIG1"})
	if got := m3.(Model).fsSig; got != "SIG1" {
		t.Fatalf("after equal poll fsSig = %q, want SIG1", got)
	}
}

// While a modal is open, a changed signature must NOT advance fsSig — the change
// stays pending so it reloads once the modal closes.
func TestPollDeferredDuringModal(t *testing.T) {
	m := ready(t)
	m2, _ := m.Update(pollResultMsg{sig: "BASE"}) // baseline
	mm := m2.(Model)
	mm.mode = modeConfirm
	m3, _ := mm.Update(pollResultMsg{sig: "CHANGED"})
	if got := m3.(Model).fsSig; got != "BASE" {
		t.Errorf("modal poll advanced fsSig to %q, want BASE (deferred)", got)
	}
}

// A reload updates fsSig atomically and keeps the selection on the same memory
// even when the underlying order changes.
func TestReloadPreservesSelectionByPath(t *testing.T) {
	mems := sampleMemories()
	m := ready(t)
	target := mems[2].Path
	m.selectByPath(target)
	if sel, ok := m.selected(); !ok || sel.Path != target {
		t.Fatalf("setup: selected %q (ok=%v), want %q", sel.Path, ok, target)
	}

	reordered := make([]memory.Memory, len(mems))
	for i := range mems {
		reordered[i] = mems[len(mems)-1-i]
	}
	m2, _ := m.Update(reloadMsg{mems: reordered, sig: "NEWSIG"})
	got := m2.(Model)
	if got.fsSig != "NEWSIG" {
		t.Errorf("fsSig = %q, want NEWSIG", got.fsSig)
	}
	if sel, ok := got.selected(); !ok || sel.Path != target {
		t.Errorf("selection after reload = %q (ok=%v), want %q", sel.Path, ok, target)
	}
}

// A reload error keeps the existing memories and surfaces a status message.
func TestReloadErrorKeepsMemories(t *testing.T) {
	m := ready(t)
	before := len(m.memories)
	m2, _ := m.Update(reloadMsg{err: errors.New("boom")})
	got := m2.(Model)
	if len(got.memories) != before {
		t.Errorf("memories changed on error: %d -> %d", before, len(got.memories))
	}
	if got.status == "" {
		t.Error("expected a status message on reload error")
	}
}
