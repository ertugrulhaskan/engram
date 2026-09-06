package tui

import (
	"reflect"
	"strings"
	"testing"
)

func rowText(rows []diffRow) string {
	var b strings.Builder
	for _, r := range rows {
		switch r.op {
		case diffYours:
			b.WriteString("-" + r.text + "\n")
		case diffTheirs:
			b.WriteString("+" + r.text + "\n")
		case diffElide:
			b.WriteString("⋮\n")
		case diffMore:
			b.WriteString("…\n")
		default:
			b.WriteString(" " + r.text + "\n")
		}
	}
	return b.String()
}

// A line diff keeps the common lines and marks each side's own, in order.
func TestDiffLines(t *testing.T) {
	got := rowText(diffLines(
		[]string{"a", "mine", "c"},
		[]string{"a", "theirs", "c"}))
	want := " a\n-mine\n+theirs\n c\n"
	if got != want {
		t.Errorf("diff =\n%swant\n%s", got, want)
	}

	// A pure insertion keeps every common line.
	got = rowText(diffLines([]string{"a", "b"}, []string{"a", "new", "b"}))
	if want := " a\n+new\n b\n"; got != want {
		t.Errorf("insertion =\n%swant\n%s", got, want)
	}

	// One side empty: everything belongs to the other.
	if got := rowText(diffLines(nil, []string{"x"})); got != "+x\n" {
		t.Errorf("empty yours = %q", got)
	}
	if got := rowText(diffLines([]string{"x"}, nil)); got != "-x\n" {
		t.Errorf("empty theirs = %q", got)
	}
	// Identical input has no marked rows.
	for _, r := range diffLines([]string{"a", "b"}, []string{"a", "b"}) {
		if r.op != diffEqual {
			t.Errorf("identical sides produced %v", r.op)
		}
	}
}

// Unchanged runs longer than the context collapse to one elision that counts
// the lines it stands for; the changes and their context survive.
func TestCollapseDiff(t *testing.T) {
	yours := []string{"1", "2", "3", "4", "5", "6", "7", "8", "mine"}
	theirs := []string{"1", "2", "3", "4", "5", "6", "7", "8", "theirs"}
	rows := collapseDiff(diffLines(yours, theirs), 2)
	got := rowText(rows)
	want := "⋮\n 7\n 8\n-mine\n+theirs\n"
	if got != want {
		t.Errorf("collapsed =\n%swant\n%s", got, want)
	}
	if rows[0].n != 6 {
		t.Errorf("elision stands for %d lines, want 6", rows[0].n)
	}
}

// The cap trims the tail and counts every line it dropped, elisions included.
func TestCapDiff(t *testing.T) {
	rows := []diffRow{
		{op: diffEqual, text: "a"},
		{op: diffYours, text: "b"},
		{op: diffElide, n: 5},
		{op: diffTheirs, text: "c"},
	}
	// Keeping 2 rows drops "b", the elision standing for 5, and "c" — 7 lines.
	// The tail is diffMore, not diffElide: what it hides includes changes, so
	// calling it "unchanged" would tell the user the remainder agrees.
	got := capDiff(rows, 2)
	if len(got) != 2 || got[1].op != diffMore || got[1].n != 7 {
		t.Errorf("capDiff = %+v, want 2 rows ending in a diffMore of 7", got)
	}
	if same := capDiff(rows, 9); len(same) != len(rows) {
		t.Errorf("a diff within the cap must be untouched, got %d rows", len(same))
	}
}

// alignDiff reports "no differences" for sides whose content matches — a
// resolve is reachable on an anchor difference alone.
func TestAlignDiffNoChange(t *testing.T) {
	rows, changed := alignDiff([]string{"same"}, []string{"same"}, 2)
	if changed || rows != nil {
		t.Errorf("identical sides: rows=%+v changed=%v, want none/false", rows, changed)
	}
	rows, changed = alignDiff([]string{"a"}, []string{"b"}, 2)
	if !changed || len(rows) != 2 {
		t.Errorf("differing sides: rows=%+v changed=%v", rows, changed)
	}
}

// The alignment falls back to whole-side replacement rather than stalling on a
// pathological pair, and still accounts for every line.
func TestDiffOversizeFallback(t *testing.T) {
	n := 600 // 600*600 = 360_000 cells > maxDiffCells
	a := make([]string, n)
	b := make([]string, n)
	for i := range a {
		a[i] = "a" + string(rune('0'+i%10))
		b[i] = "b" + string(rune('0'+i%10))
	}
	rows := diffLines(a, b)
	if len(rows) != 2*n {
		t.Fatalf("oversize diff produced %d rows, want %d", len(rows), 2*n)
	}
	var yours, theirs int
	for _, r := range rows {
		switch r.op {
		case diffYours:
			yours++
		case diffTheirs:
			theirs++
		}
	}
	if yours != n || theirs != n {
		t.Errorf("oversize diff: %d yours / %d theirs, want %d each", yours, theirs, n)
	}
}

// Every op renders with its own gutter mark, and the elision names its count.
func TestResolveModalRendersDiff(t *testing.T) {
	m := ready(t)
	m.mode = modeResolveConfirm
	m.resolvePath, m.resolveTmp = "/x/mem.md", "/tmp/merge.md"
	m.resolveRows = []diffRow{
		{op: diffElide, n: 4},
		{op: diffEqual, text: "context line"},
		{op: diffYours, text: "wrap it in a transaction"},
		{op: diffTheirs, text: "wrap it, and keep a down step"},
	}
	out := m.View()
	for _, want := range []string{
		"resolve — both sides moved",
		"− yours · + the team store",
		"⋮ 4 unchanged lines",
		"context line",
		"− wrap it in a transaction",
		"+ wrap it, and keep a down step",
		"↵ open $EDITOR",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("resolve modal missing %q:\n%s", want, out)
		}
	}

	// Identical sides say so instead of showing an empty diff.
	m.resolveRows, m.resolveSame = nil, resolveIdentical
	if out := m.View(); !strings.Contains(out, "only the sync anchor differs") {
		t.Errorf("identical sides should say so:\n%s", out)
	}
}

// A single line is not pluralized — pinned where it is still reachable.
// collapseDiff no longer elides a run of one (the row an elision costs is the
// row it would save; see TestCollapseKeepsALoneLine), so the case this test
// used to construct now returns the line itself. That is asserted here too, so
// the two tests fail together if the rule is reverted. diffMore still counts
// down to one, and pluralLine also serves the pull confirm.
func TestElisionPlural(t *testing.T) {
	rows := collapseDiff(diffLines(
		[]string{"1", "2", "3", "4", "mine"},
		[]string{"1", "2", "3", "4", "theirs"}), 3)
	for _, r := range rows {
		if r.op == diffElide && r.n == 1 {
			t.Fatalf("an elision standing for one line costs the row it saves: %+v", rows)
		}
	}
	if got := pluralLine(1, "1 unchanged line", "%d unchanged lines"); got != "1 unchanged line" {
		t.Errorf("singular = %q", got)
	}
	if got := pluralLine(4, "1 unchanged line", "%d unchanged lines"); got != "4 unchanged lines" {
		t.Errorf("plural = %q", got)
	}
	if got := pluralLine(1, "1 more line — see it all in $EDITOR", "%d more lines — see them all in $EDITOR"); got != "1 more line — see it all in $EDITOR" {
		t.Errorf("diffMore singular = %q", got)
	}
	_ = reflect.DeepEqual
}

// A capped diff never calls the hidden remainder "unchanged": the rows past
// the cap include changes, and the confirm has to say so.
func TestCapDiffTailIsNotCalledUnchanged(t *testing.T) {
	var yours, theirs []string
	for i := 0; i < 12; i++ {
		yours = append(yours, "mine "+string(rune('a'+i)))
		theirs = append(theirs, "theirs "+string(rune('a'+i)))
	}
	m := ready(t)
	m.mode = modeResolveConfirm
	m.setResolveSides(yours, theirs, false)
	if !m.resolveChanged || len(m.resolveRows) != m.resolveDiffRows() {
		t.Fatalf("rows=%d changed=%v, want %d rows", len(m.resolveRows), m.resolveChanged, m.resolveDiffRows())
	}
	if last := m.resolveRows[len(m.resolveRows)-1]; last.op != diffMore {
		t.Fatalf("last row op = %v, want diffMore", last.op)
	}
	out := m.View()
	if strings.Contains(out, "unchanged line") {
		t.Errorf("a capped diff must not claim the hidden tail is unchanged:\n%s", out)
	}
	if !strings.Contains(out, "more lines — see them all in $EDITOR") {
		t.Errorf("capped diff missing the honest tail line:\n%s", out)
	}
}

// An elision must buy a row to be worth drawing. A run of exactly one skipped
// line costs the same row as the "⋮ 1 unchanged line" that replaced it, so the
// dialog paid the same and showed less. With ctx=2, two changes five lines
// apart leave exactly one line outside both context windows — the case.
func TestCollapseKeepsALoneLine(t *testing.T) {
	rows := []diffRow{
		{op: diffYours, text: "Y"},
		{op: diffEqual, text: "a"}, {op: diffEqual, text: "b"},
		{op: diffEqual, text: "c"},
		{op: diffEqual, text: "d"}, {op: diffEqual, text: "e"},
		{op: diffYours, text: "Y"},
	}
	got := collapseDiff(rows, 2)
	if len(got) != len(rows) {
		t.Fatalf("collapseDiff = %d rows, want %d unchanged", len(got), len(rows))
	}
	for i, r := range got {
		if r.op == diffElide {
			t.Fatalf("row %d is an elision standing for %d line(s) — it costs the row it saves", i, r.n)
		}
		if r.text != rows[i].text {
			t.Errorf("row %d = %q, want %q", i, r.text, rows[i].text)
		}
	}
	// Two or more still collapse: that is where the row is actually saved.
	long := []diffRow{{op: diffYours, text: "Y"}}
	for i := 0; i < 8; i++ {
		long = append(long, diffRow{op: diffEqual, text: "x"})
	}
	long = append(long, diffRow{op: diffYours, text: "Y"})
	if got := collapseDiff(long, 2); len(got) >= len(long) {
		t.Errorf("a run of 4 skipped lines did not collapse: %d rows in, %d out", len(long), len(got))
	}
}
