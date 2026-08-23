package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ertugrulhaskan/engram/internal/config"
	"github.com/ertugrulhaskan/engram/internal/memory"
)

// markFixture spans two types with more than one memory each. The shared
// sampleMemories() carries exactly one of every type, which would let a
// per-row implementation pass a "whole type" test by accident — this one
// cannot.
func markFixture(t *testing.T) Model {
	t.Helper()
	mems := []memory.Memory{
		mem("terse-prose", "keep explanations short", memory.TypeUser, "acme", "/p/acme/memory/u1.md", "2024-01-02"),
		mem("no-attribution", "no trailers on commits", memory.TypeFeedback, "acme", "/p/acme/memory/f1.md", "2024-01-03"),
		mem("small-steps", "review each step before the next", memory.TypeFeedback, "acme", "/p/acme/memory/f2.md", "2024-01-04"),
		mem("blunt-verdicts", "lead with the verdict", memory.TypeFeedback, "widgets", "/p/widgets/memory/f3.md", "2024-01-05"),
		mem("roadmap-doc", "sequencing lives here", memory.TypeReference, "widgets", "/p/widgets/memory/r1.md", "2024-01-06"),
	}
	m, _ := New(mems, nil, nil, config.Config{}).Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	return m.(Model)
}

// selectType cycles `t` until the filter reads want, so the tests below spell
// the keystrokes a user actually makes instead of poking typeIdx directly.
func selectType(t *testing.T, m tea.Model, want memory.Type) tea.Model {
	t.Helper()
	for i := 0; i <= len(typeCycle); i++ {
		if typeCycle[m.(Model).typeIdx] == want {
			return m
		}
		m = typeRunes(m, "t")
	}
	t.Fatalf("type %q never came up in the t cycle", want)
	return m
}

// markedPaths returns the marked set as a sorted, comparable string.
func markedPaths(m Model) string {
	var got []string
	for p := range m.marks {
		got = append(got, p)
	}
	sortStrings(got)
	return strings.Join(got, ",")
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

// With no filter shaping the list, `a` marks the whole source.
func TestMarkAllMarksEveryListedMemory(t *testing.T) {
	var m tea.Model = markFixture(t)
	m = typeRunes(m, "a")

	if n := len(m.(Model).marks); n != 5 {
		t.Errorf("marks = %d after a, want all 5", n)
	}
}

// The feature ENGR-17 asks for: the type filter is the selection, so `t` to a
// type then `a` marks that type and nothing else.
func TestMarkAllMarksTheWholeTypeFilter(t *testing.T) {
	var m tea.Model = markFixture(t)
	m = selectType(t, m, memory.TypeFeedback)
	m = typeRunes(m, "a")

	want := "/p/acme/memory/f1.md,/p/acme/memory/f2.md,/p/widgets/memory/f3.md"
	if got := markedPaths(m.(Model)); got != want {
		t.Errorf("marked set after t→feedback, a:\n got  %s\n want %s", got, want)
	}
}

// `a` scales space's toggle up to the list: pressing it on a fully-marked list
// clears that list rather than doing nothing.
func TestMarkAllTogglesTheListedSetOff(t *testing.T) {
	var m tea.Model = markFixture(t)
	m = typeRunes(m, "a")
	if n := len(m.(Model).marks); n != 5 {
		t.Fatalf("setup: marks = %d, want 5", n)
	}

	m = typeRunes(m, "a")
	if n := len(m.(Model).marks); n != 0 {
		t.Errorf("marks = %d after a twice, want 0", n)
	}
}

// A partly-marked list completes to fully marked. Toggling off here would lose
// the marks the user just made by hand, which is the opposite of the intent.
func TestMarkAllCompletesAPartlyMarkedList(t *testing.T) {
	var m tea.Model = markFixture(t)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace}) // mark one row by hand
	if n := len(m.(Model).marks); n != 1 {
		t.Fatalf("setup: marks = %d, want 1", n)
	}

	m = typeRunes(m, "a")
	if n := len(m.(Model).marks); n != 5 {
		t.Errorf("marks = %d after a on a partly-marked list, want 5", n)
	}
}

// Marks made under one filter survive marking and unmarking under another. A
// batch assembled across two types is the whole reason the marked set is not
// just "whatever the filters currently show".
func TestMarkAllLeavesMarksOutsideTheListAlone(t *testing.T) {
	var m tea.Model = markFixture(t)
	m = selectType(t, m, memory.TypeFeedback)
	m = typeRunes(m, "a") // 3 feedback marked
	feedback := markedPaths(m.(Model))

	m = selectType(t, m, memory.TypeReference)
	m = typeRunes(m, "a") // + 1 reference
	if n := len(m.(Model).marks); n != 4 {
		t.Fatalf("marks = %d after marking a second type, want 4", n)
	}

	m = typeRunes(m, "a") // toggle the reference list back off
	if got := markedPaths(m.(Model)); got != feedback {
		t.Errorf("unmarking the reference list disturbed the feedback marks:\n got  %s\n want %s", got, feedback)
	}
}

// The search filter narrows the list too, so it narrows what `a` marks.
func TestMarkAllRespectsTheSearchFilter(t *testing.T) {
	m := markFixture(t)
	m.search.SetValue("blunt")
	m.rebuildRows()

	var tm tea.Model = m
	tm = typeRunes(tm, "a")

	want := "/p/widgets/memory/f3.md"
	if got := markedPaths(tm.(Model)); got != want {
		t.Errorf("marked set under search %q:\n got  %s\n want %s", "blunt", got, want)
	}
}

// Typing into the search box must reach the input, not the mark-all key. `a` is
// a plain rune, so this is the regression the binding invites.
func TestMarkAllDoesNotFireWhileSearching(t *testing.T) {
	var m tea.Model = markFixture(t)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m = typeRunes(m, "a")

	got := m.(Model)
	if n := len(got.marks); n != 0 {
		t.Errorf("marks = %d while typing in the search box, want 0", n)
	}
	if v := got.search.Value(); v != "a" {
		t.Errorf("search value = %q, want %q — the keystroke never reached the input", v, "a")
	}
}

// Marking is memories-only, exactly as space is: the read-only files source has
// nothing to promote, so `a` there must not accumulate state.
func TestMarkAllIgnoredOutsideMemories(t *testing.T) {
	var m tea.Model = ready(t)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	m = typeRunes(m, "/files")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if got := m.(Model); got.srcKind != srcFiles {
		t.Fatalf("setup: srcKind=%v, want srcFiles", got.srcKind)
	}

	m = typeRunes(m, "a")
	if n := len(m.(Model).marks); n != 0 {
		t.Errorf("a marked %d rows in the files source, want 0", n)
	}
}

// The status line names the type it acted on, and discloses the running total
// whenever marks reach past the list — a batch wider than what you are looking
// at must not be a surprise at the promote confirm.
func TestMarkAllStatusNamesTypeAndTotal(t *testing.T) {
	var m tea.Model = markFixture(t)
	m = selectType(t, m, memory.TypeFeedback)
	m = typeRunes(m, "a")

	if got := m.(Model).status; got != "marked 3 feedback memories" {
		t.Errorf("status = %q, want %q", got, "marked 3 feedback memories")
	}

	m = selectType(t, m, memory.TypeReference)
	m = typeRunes(m, "a")

	want := "marked 1 reference memory · 4 marked in all"
	if got := m.(Model).status; got != want {
		t.Errorf("status = %q, want %q", got, want)
	}
}

// Without a type filter the line says "in the list" rather than naming a type,
// so it never implies a narrower selection than was made.
func TestMarkAllStatusUnfiltered(t *testing.T) {
	var m tea.Model = markFixture(t)
	m = typeRunes(m, "a")

	if got := m.(Model).status; got != "marked 5 memories in the list" {
		t.Errorf("status = %q, want %q", got, "marked 5 memories in the list")
	}
}

// An empty list has nothing to mark and says so, rather than silently doing
// nothing or reporting "marked 0".
func TestMarkAllOnAnEmptyList(t *testing.T) {
	m := markFixture(t)
	m.search.SetValue("zzzzz-no-such-memory")
	m.rebuildRows()

	var tm tea.Model = m
	tm = typeRunes(tm, "a")

	got := tm.(Model)
	if n := len(got.marks); n != 0 {
		t.Errorf("marks = %d on an empty list, want 0", n)
	}
	if got.status != "nothing in the list to mark" {
		t.Errorf("status = %q, want %q", got.status, "nothing in the list to mark")
	}
}

// evenFixture holds two types of equal size, which is what makes the
// disclosure-suppression bug reachable: unmark one type and the count left
// marked matches the count just cleared exactly.
func evenFixture(t *testing.T) Model {
	t.Helper()
	mems := []memory.Memory{
		mem("no-attribution", "no trailers", memory.TypeFeedback, "acme", "/p/acme/memory/f1.md", "2024-01-03"),
		mem("small-steps", "review each step", memory.TypeFeedback, "acme", "/p/acme/memory/f2.md", "2024-01-04"),
		mem("roadmap-doc", "sequencing", memory.TypeReference, "widgets", "/p/widgets/memory/r1.md", "2024-01-05"),
		mem("api-notes", "endpoint shapes", memory.TypeReference, "widgets", "/p/widgets/memory/r2.md", "2024-01-06"),
	}
	m, _ := New(mems, nil, nil, config.Config{}).Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	return m.(Model)
}

// Unmarking must still disclose the marks left behind when the two counts happen
// to be equal. Deriving the disclosure from a `total != n` comparison went silent
// here — precisely the case a still-armed batch most needs announcing.
func TestMarkAllUnmarkDisclosesAnEqualRemainder(t *testing.T) {
	var m tea.Model = evenFixture(t)
	m = selectType(t, m, memory.TypeFeedback)
	m = typeRunes(m, "a") // 2 feedback
	m = selectType(t, m, memory.TypeReference)
	m = typeRunes(m, "a") // + 2 reference = 4
	m = typeRunes(m, "a") // unmark the 2 reference — 2 remain, equal to the 2 cleared

	if n := len(m.(Model).marks); n != 2 {
		t.Fatalf("marks = %d, want the 2 feedback left over", n)
	}
	want := "unmarked 2 reference memories · 2 marked in all"
	if got := m.(Model).status; got != want {
		t.Errorf("status = %q, want %q", got, want)
	}
}

// After a key that means "mark them all", the count reported is the set now
// marked in the list — not how many changed. Reporting the delta describes the
// keystroke accurately and the resulting batch misleadingly.
func TestMarkAllReportsTheListNotTheDelta(t *testing.T) {
	var m tea.Model = markFixture(t)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace}) // one row marked by hand
	m = typeRunes(m, "a")

	if got := m.(Model).status; got != "marked 5 memories in the list" {
		t.Errorf("status = %q, want %q", got, "marked 5 memories in the list")
	}
}

// A search narrowing a type filter must not let the status claim the whole type.
// With t→feedback plus a search matching one of three, "marked 1 feedback memory"
// reads as "the feedback type is marked" while two are not — so the "in the list"
// hedge stays whenever a search is also shaping the list.
func TestMarkAllStatusHedgesUnderASearch(t *testing.T) {
	m := markFixture(t)
	var tm tea.Model = m
	tm = selectType(t, tm, memory.TypeFeedback)
	mm := tm.(Model)
	mm.search.SetValue("blunt")
	mm.rebuildRows()
	tm = mm
	tm = typeRunes(tm, "a")

	want := "marked 1 feedback memory in the list"
	if got := tm.(Model).status; got != want {
		t.Errorf("status = %q, want %q — 2 of the 3 feedback memories are not marked", got, want)
	}
}

// The status must name a type the way the row badges do. TypeUnknown badges as
// "other"; leaking the internal "unknown" would make the footer and the list
// disagree about the same memories.
func TestMarkAllStatusUsesTheBadgeLabel(t *testing.T) {
	mems := []memory.Memory{
		mem("legacy-a", "hand-written, no frontmatter", memory.TypeUnknown, "acme", "/p/acme/memory/x1.md", "2024-01-02"),
		mem("legacy-b", "hand-written, no frontmatter", memory.TypeUnknown, "acme", "/p/acme/memory/x2.md", "2024-01-03"),
	}
	mm, _ := New(mems, nil, nil, config.Config{}).Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	var tm tea.Model = mm
	tm = selectType(t, tm, memory.TypeUnknown)
	tm = typeRunes(tm, "a")

	want := "marked 2 other memories"
	if got := tm.(Model).status; got != want {
		t.Errorf("status = %q, want %q (typeLabel renders TypeUnknown as %q)", got, want, typeLabel(memory.TypeUnknown))
	}
}
