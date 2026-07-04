package tui

import (
	"sort"
	"strings"
	"time"

	"github.com/ertugrulhaskan/engram/internal/memory"
	"github.com/ertugrulhaskan/engram/internal/plan"
)

// rowKind distinguishes the three kinds of display rows in the list.
type rowKind int

const (
	rowMemory rowKind = iota
	rowHeader
	rowSpacer
)

type row struct {
	kind  rowKind
	item  Item
	label string // header label
	color string // header color (hex)
	count int    // header group size
}

// Item is the source-agnostic unit the list and preview render. Each source
// (memories, plans) maps its records into Items; presentation (badge text,
// colors, group labels) is resolved by the mapper so the renderers stay generic.
type Item struct {
	Title    string
	Body     string // markdown for the preview
	Raw      string // fallback when Body is empty
	Path     string // identity: selection, edit, delete
	Modified time.Time

	Badge      string // bracket label, e.g. "user"/"project"; "" = no badge column
	BadgeColor string // hex
	SyncBadge  string // team-sync glyph (✓/●/!); "" = no sync column for this row
	SyncColor  string // hex for the sync glyph
	GroupKey   string // "" = flat (no group headers)
	GroupLabel string // header text for the first row of a group
	GroupColor string // header color (hex)
	Right      string // right-aligned column text (project when grouped by type, or date)
	Context    string // preview meta context (project name, or "plan")
	MemDir     string // memory dir for new/index/drift; "" for plans
	ProjectDir string // decoded project dir, for launching an assistant in context; "" for plans
	Kind       string // "memory" | "plan" — palette tag + feature gating
}

// fuzzyScore reports whether all runes of query appear in order in s (case-
// insensitive), scoring by match span so tighter matches rank higher.
func fuzzyScore(query, s string) (int, bool) {
	q := []rune(strings.ToLower(query))
	if len(q) == 0 {
		return 0, true
	}
	text := []rune(strings.ToLower(s))
	qi, first, last := 0, -1, -1
	for ti := 0; ti < len(text) && qi < len(q); ti++ {
		if text[ti] == q[qi] {
			if first < 0 {
				first = ti
			}
			last = ti
			qi++
		}
	}
	if qi < len(q) {
		return 0, false
	}
	return last - first, true
}

// --- list model ---

// rebuildRows recomputes display rows from the active source: it maps records to
// Items (the source applies its own grouping/ordering), applies the generic
// search filter, then builds header/spacer/item rows by GroupKey.
func (m *Model) rebuildRows() {
	items := m.activeItems()

	if q := strings.ToLower(strings.TrimSpace(m.search.Value())); q != "" {
		var f []Item
		for _, it := range items {
			if strings.Contains(strings.ToLower(it.Title+" "+it.Context+" "+it.Body), q) {
				f = append(f, it)
			}
		}
		items = f
	}

	counts := map[string]int{}
	for _, it := range items {
		counts[it.GroupKey]++
	}

	var rows []row
	prevKey := "\x00sentinel"
	for _, it := range items {
		if it.GroupKey != "" && it.GroupKey != prevKey {
			if len(rows) > 0 {
				rows = append(rows, row{kind: rowSpacer})
			}
			rows = append(rows, row{kind: rowHeader, label: it.GroupLabel, color: it.GroupColor, count: counts[it.GroupKey]})
		}
		prevKey = it.GroupKey
		rows = append(rows, row{kind: rowMemory, item: it})
	}

	m.rows = rows
	if m.cursor >= len(rows) || m.cursor < 0 || rows[clampIdx(m.cursor, len(rows))].kind != rowMemory {
		m.cursor = m.firstMemRow()
	}
	m.ensureVisible()
	m.syncPreview()
}

// activeItems returns the Items for the source currently being browsed.
func (m Model) activeItems() []Item {
	switch m.srcKind {
	case srcPlans:
		return m.planItems()
	case srcFiles:
		return m.docItems()
	default:
		return m.memoryItems()
	}
}

// docItems maps the read-only Claude docs (CLAUDE.md / MEMORY.md) into Items,
// grouped by scope (global first, then per project) — the source is already
// sorted that way so the groups stay contiguous. These rows are view-only; the
// `e`/`d` handlers point the user at @Claude instead.
func (m Model) docItems() []Item {
	t := m.theme()
	items := make([]Item, 0, len(m.docs))
	colorFor := t.groupColorer()
	for _, d := range m.docs {
		badge, bcolor := "rules", t.TFeedback
		if d.Kind == memory.DocIndex {
			badge, bcolor = "index", t.TReference
		}
		items = append(items, Item{
			Title: d.Title, Body: d.Body, Raw: d.Body, Path: d.Path, Modified: d.Modified,
			Badge: badge, BadgeColor: bcolor,
			GroupKey: d.Scope, GroupLabel: d.Scope, GroupColor: colorFor(d.Scope),
			Right: humanizeSince(d.Modified), Context: d.Scope,
			MemDir: d.MemoryDir, ProjectDir: d.ProjectDir, Kind: string(d.Kind),
		})
	}
	return items
}

// memoryItems maps memories into Items, applying the active type filter and
// grouping (project or type) and resolving badge/group colors from the theme.
func (m Model) memoryItems() []Item {
	t := m.theme()
	tf := typeCycle[m.typeIdx]
	byType := m.groupBy == groupType

	var sub []memory.Memory
	for _, mm := range m.memories {
		if tf == "" || mm.Type == tf {
			sub = append(sub, mm)
		}
	}
	sortForGroup(sub, m.groupBy)

	items := make([]Item, 0, len(sub))
	colorFor := t.groupColorer()
	for _, mm := range sub {
		key := groupKeyOf(mm, m.groupBy)
		label, color := mm.Project.Name, colorFor(key)
		right := ""
		if byType {
			label, color = typeLabel(mm.Type), t.typeColor(mm.Type)
			right = "· " + mm.Project.Name
		}
		sg, sc, _ := syncBadge(m.syncStates[mm.Path]) // "" for personal/unshared rows
		items = append(items, Item{
			Title: mm.Title, Body: mm.Body, Raw: mm.Raw, Path: mm.Path, Modified: mm.Modified,
			Badge: typeName(mm.Type), BadgeColor: t.typeColor(mm.Type),
			SyncBadge: sg, SyncColor: sc,
			GroupKey: key, GroupLabel: label, GroupColor: color,
			Right: right, Context: mm.Project.Name, MemDir: mm.Project.MemoryDir, ProjectDir: mm.Project.Dir, Kind: "memory",
		})
	}
	return items
}

// planItems maps plans into Items grouped by recency (Today / This week /
// Older), mirroring how memories group by project: colored group headers with
// counts and the modified date in the right column. Plans have no
// badge/type/project. Sorting newest-first here makes the buckets contiguous
// regardless of the order plans arrive in (the same guarantee memoryItems gets
// from sortForGroup).
func (m Model) planItems() []Item {
	t := m.theme()
	plans := append([]plan.Plan(nil), m.plans...)
	sort.SliceStable(plans, func(i, j int) bool { return plans[i].Modified.After(plans[j].Modified) })

	items := make([]Item, 0, len(plans))
	colorFor := t.groupColorer()
	for _, p := range plans {
		key, label := recencyBucket(p.Modified)
		items = append(items, Item{
			Title: p.Title, Body: p.Body, Raw: p.Body, Path: p.Path, Modified: p.Modified,
			GroupKey: key, GroupLabel: label, GroupColor: colorFor(key),
			Right: humanizeSince(p.Modified), Context: "plan", Kind: "plan",
		})
	}
	return items
}

// recencyBucket buckets a plan by how recently it was modified into three
// coarse spans: Today (<24h), This week (<7d), and Older. The right-column date
// (humanizeSince) is finer-grained, so a row can read e.g. "3w ago" under the
// "Older" header — consistent, just more precise than the bucket.
func recencyBucket(mod time.Time) (key, label string) {
	if mod.IsZero() {
		return "older", "Older"
	}
	switch d := time.Since(mod); {
	case d < 24*time.Hour:
		return "today", "Today"
	case d < 7*24*time.Hour:
		return "week", "This week"
	default:
		return "older", "Older"
	}
}

// --- grouping helpers ---

func groupKeyOf(mm memory.Memory, by groupMode) string {
	if by == groupType {
		return string(mm.Type)
	}
	return mm.Project.Name
}

func sortForGroup(mems []memory.Memory, by groupMode) {
	sort.SliceStable(mems, func(i, j int) bool {
		ki, kj := groupKeyOf(mems[i], by), groupKeyOf(mems[j], by)
		if ki != kj {
			return ki < kj
		}
		// Within a project group, cluster by type before falling back to title.
		if by == groupProject {
			if ri, rj := typeRank(mems[i].Type), typeRank(mems[j].Type); ri != rj {
				return ri < rj
			}
		}
		return mems[i].Title < mems[j].Title
	})
}

// typeRank is the within-group ordering of memory types: project, feedback,
// user, reference, then everything else.
func typeRank(t memory.Type) int {
	switch t {
	case memory.TypeProject:
		return 0
	case memory.TypeFeedback:
		return 1
	case memory.TypeUser:
		return 2
	case memory.TypeReference:
		return 3
	default:
		return 4
	}
}
