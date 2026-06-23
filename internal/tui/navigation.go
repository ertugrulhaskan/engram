package tui

import (
	"strings"

	"github.com/ertugrulhaskan/engram/internal/memory"
)

// switchSource changes the active source, remembering the per-source cursor and
// clearing the (per-source) filter.
func (m *Model) switchSource(k srcKind) {
	if k == m.srcKind {
		return
	}
	m.cursors[m.srcKind] = m.cursor
	m.srcKind = k
	m.search.SetValue("")
	m.cursor = m.cursors[k]
	m.rebuildRows() // clamps the remembered cursor if it's now out of range
}

func (m *Model) firstMemRow() int {
	for i, r := range m.rows {
		if r.kind == rowMemory {
			return i
		}
	}
	return 0
}

// move steps the cursor by delta, skipping header and spacer rows.
func (m *Model) move(delta int) {
	i := m.cursor
	for {
		j := i + delta
		if j < 0 || j >= len(m.rows) {
			return
		}
		i = j
		if m.rows[i].kind == rowMemory {
			m.cursor = i
			m.ensureVisible()
			m.syncPreview()
			return
		}
	}
}

// page jumps the cursor about one screen in dir (-1 up, +1 down), snapping to
// the nearest memory row.
func (m *Model) page(dir int) {
	if len(m.rows) == 0 {
		return
	}
	h := m.listRows()
	if h < 1 {
		h = 1
	}
	target := m.cursor + dir*h
	if target < 0 {
		target = 0
	}
	if target > len(m.rows)-1 {
		target = len(m.rows) - 1
	}
	j := target
	for j >= 0 && j < len(m.rows) && m.rows[j].kind != rowMemory { // prefer dir
		j += dir
	}
	if j < 0 || j >= len(m.rows) { // fall back to the opposite direction
		for j = target; j >= 0 && j < len(m.rows) && m.rows[j].kind != rowMemory; j -= dir {
		}
	}
	if j >= 0 && j < len(m.rows) && m.rows[j].kind == rowMemory {
		m.cursor = j
		m.ensureVisible()
		m.syncPreview()
	}
}

// shownCount is the number of memory rows currently displayed (post-filter).
func (m Model) shownCount() int {
	n := 0
	for _, r := range m.rows {
		if r.kind == rowMemory {
			n++
		}
	}
	return n
}

func (m *Model) ensureVisible() {
	h := m.listRows()
	if h < 1 {
		return
	}
	if m.cursor < m.top {
		m.top = m.cursor
	}
	if m.cursor >= m.top+h {
		m.top = m.cursor - h + 1
	}
	// Pull the group header above the cursor into view when it fits.
	if m.cursor > 0 && m.rows[m.cursor-1].kind == rowHeader && m.cursor-1 < m.top {
		m.top = m.cursor - 1
	}
	if m.top < 0 {
		m.top = 0
	}
}

func (m Model) selected() (Item, bool) {
	if m.cursor >= 0 && m.cursor < len(m.rows) && m.rows[m.cursor].kind == rowMemory {
		return m.rows[m.cursor].item, true
	}
	return Item{}, false
}

// selectByPath moves the cursor to the item with the given path, if it's still
// present. If not found (e.g. it was deleted) the clamped cursor is left as-is.
func (m *Model) selectByPath(path string) {
	for i, r := range m.rows {
		if r.kind == rowMemory && r.item.Path == path {
			m.cursor = i
			m.ensureVisible()
			m.syncPreview()
			return
		}
	}
}

func (m Model) currentMemDir() string {
	if it, ok := m.selected(); ok && it.MemDir != "" {
		return it.MemDir
	}
	if len(m.memories) > 0 {
		return m.memories[0].Project.MemoryDir
	}
	return ""
}

func (m *Model) syncPreview() {
	if !m.ready {
		return
	}
	it, ok := m.selected()
	if !ok {
		m.viewport.SetContent("")
		return
	}
	// Index drift applies to memories only; refresh the flag when the selected
	// project changes (cached by memory dir so it only touches disk on a switch).
	if it.Kind == "memory" {
		if it.MemDir != m.driftDir {
			m.driftDir = it.MemDir
			un, dang, err := memory.IndexDrift(it.MemDir)
			if err == nil {
				m.driftUnindexed, m.driftDangling = len(un), len(dang)
			} else {
				m.driftUnindexed, m.driftDangling = 0, 0
			}
			m.driftOut = m.driftUnindexed > 0 || m.driftDangling > 0
		}
	} else {
		m.driftOut = false
	}
	if m.previewCache == nil {
		m.previewCache = map[string]string{}
	}
	if cached, ok := m.previewCache[it.Path]; ok {
		m.viewport.SetContent(cached)
		m.viewport.GotoTop()
		return
	}
	// Decide the empty-body fallback before stripping, so a body that is only a
	// heading renders as empty rather than falling back to the raw frontmatter.
	body := it.Body
	if body == "" {
		body = it.Raw
	}
	body = stripFirstHeading(body)
	rendered := body
	if m.renderer != nil {
		if out, err := m.renderer.Render(body); err == nil {
			rendered = out
		}
	}
	// Glamour pads its output with leading/trailing blank lines; trim them so
	// the viewport only scrolls over real content.
	rendered = trimBlankLines(rendered)
	m.previewCache[it.Path] = rendered
	m.viewport.SetContent(rendered)
	m.viewport.GotoTop()
}

// trimBlankLines drops leading and trailing all-whitespace lines.
func trimBlankLines(s string) string {
	lines := strings.Split(s, "\n")
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	return strings.Join(lines, "\n")
}
