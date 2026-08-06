package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ertugrulhaskan/engram/internal/team"
)

// storeTimeMsg carries the store's last commit time for one engram id, fetched
// off the UI loop. err non-nil means the stamp stays omitted.
type storeTimeMsg struct {
	id  string
	t   time.Time
	err error
}

// storeTimeCmd asks git (via team.StoreLastChange) when the store copy of the
// memory last changed. It runs as a tea.Cmd so the UI never blocks on git.
func storeTimeCmd(id string) tea.Cmd {
	return func() tea.Msg {
		ts, err := team.StoreLastChange(id)
		return storeTimeMsg{id: id, t: ts, err: err}
	}
}

// maybeFetchStoreTime returns a fetch command when the selected row is a
// behind memory whose store timestamp hasn't been asked for yet, marking it
// asked so one selection fires at most one git call. Only StateIncoming needs
// the store side's time — every other state stamps from local knowledge.
func (m *Model) maybeFetchStoreTime() tea.Cmd {
	it, ok := m.selected()
	if !ok || it.Sync != team.StateIncoming || it.SyncID == "" {
		return nil
	}
	if m.storeTimeAsked[it.SyncID] {
		return nil
	}
	if m.storeTimeAsked == nil {
		m.storeTimeAsked = map[string]bool{}
	}
	m.storeTimeAsked[it.SyncID] = true
	return storeTimeCmd(it.SyncID)
}

// withStoreTimeFetch piggybacks the lazy store-timestamp fetch onto an update
// whose result may have changed the selection (keys, resize, reload). It is
// the single funnel, so no navigation path needs to remember to fetch.
func withStoreTimeFetch(nm tea.Model, cmd tea.Cmd) (tea.Model, tea.Cmd) {
	mm, ok := nm.(Model)
	if !ok {
		return nm, cmd
	}
	if fetch := mm.maybeFetchStoreTime(); fetch != nil {
		return mm, tea.Batch(cmd, fetch)
	}
	return mm, cmd
}
