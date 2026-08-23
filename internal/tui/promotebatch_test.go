package tui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/ertugrulhaskan/engram/internal/secrets"
)

func batchFixture() []batchItem {
	return []batchItem{
		{path: "/p/a.md", title: "a", key: "github.com/acme/app"},
		{path: "/p/b.md", title: "b", key: "github.com/acme/app"},
		{path: "/p/c.md", title: "c", key: "github.com/acme/lib"},
		{path: "/p/d.md", title: "d", key: ""}, // project has no git remote
	}
}

// "their own projects" sends each memory to its own key, and a memory whose
// project has no remote falls to global — there is nowhere else it can go.
func TestResolvePlacements(t *testing.T) {
	own := resolvePlacements(batchFixture(), true)
	want := []string{"github.com/acme/app", "github.com/acme/app", "github.com/acme/lib", "global"}
	for i, w := range want {
		if own[i].placement != w {
			t.Errorf("own[%d] placement = %q, want %q", i, own[i].placement, w)
		}
	}
	for i, b := range resolvePlacements(batchFixture(), false) {
		if b.placement != "global" {
			t.Errorf("global[%d] placement = %q, want global", i, b.placement)
		}
	}
}

// The scope row has to state the real spread, since "their own projects" is
// otherwise an invisible fan-out across several stores.
func TestBatchScopeSub(t *testing.T) {
	keys, remoteless := distinctKeys(batchFixture())
	if keys != 2 || remoteless != 1 {
		t.Fatalf("distinctKeys = (%d, %d), want (2, 1)", keys, remoteless)
	}
	sub := batchScopeSub(batchFixture())
	if !strings.Contains(sub, "2 project keys") || !strings.Contains(sub, "no remote") {
		t.Errorf("scope sub = %q, want the key count and the remote-less disclosure", sub)
	}
}

// The scope modal names the batch and offers the per-project row.
func TestBatchScopeModal(t *testing.T) {
	m := ready(t)
	m.batchItems = batchFixture()
	m.mode = modePromoteScope
	out := ansi.Strip(m.scopeModal())

	if !strings.Contains(out, "promote 4 memories") {
		t.Errorf("header missing the count:\n%s", out)
	}
	if !strings.Contains(out, "their own projects") {
		t.Errorf("missing the per-project row:\n%s", out)
	}
	if !strings.Contains(out, "one commit") {
		t.Errorf("mechanics line should say the batch is one commit:\n%s", out)
	}
}

// A batch whose marked memories all lack a remote offers only global, and says
// why — the same disclosure the single-memory path already makes.
func TestBatchScopeModalNoRemotes(t *testing.T) {
	m := ready(t)
	m.batchItems = []batchItem{{path: "/p/a.md", title: "a"}, {path: "/p/b.md", title: "b"}}
	m.promoteCursor = 1
	m.mode = modePromoteScope
	out := ansi.Strip(m.scopeModal())
	if !strings.Contains(out, "none of these projects has a git remote") {
		t.Errorf("modal must disclose why only global is offered:\n%s", out)
	}
	if strings.Contains(out, "their own projects") {
		t.Errorf("offered a per-project row with no keys available:\n%s", out)
	}
}

func walkModel(t *testing.T, policy string) Model {
	t.Helper()
	m := ready(t)
	m.scanAction = policy
	m.scanAccepted = []batchItem{{path: "/p/clean.md", title: "clean", placement: "global"}}
	m.scanFlagged = []flaggedMemory{
		{item: batchItem{path: "/p/x.md", title: "x", placement: "global"}, findings: []secrets.Finding{{Line: 1, Rule: "aws-key", Match: "AKIA****"}}},
		{item: batchItem{path: "/p/y.md", title: "y", placement: "global"}, findings: []secrets.Finding{{Line: 2, Rule: "entropy", Match: "Xk9m****"}}},
	}
	m.scanIdx = 0
	m.mode = modeSecretWarn
	m.loadFlagged()
	return m
}

// y includes the flagged memory, n skips it, and the walk advances one memory at
// a time — the per-memory decision the batch promotes under.
func TestBatchWalkIncludeThenSkip(t *testing.T) {
	var tm tea.Model = walkModel(t, "block")

	tm, _ = tm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	mid := tm.(Model)
	if mid.scanIdx != 1 || len(mid.scanAccepted) != 2 {
		t.Fatalf("after y: idx=%d accepted=%d, want 1 and 2", mid.scanIdx, len(mid.scanAccepted))
	}
	if mid.secretTitle != "y" {
		t.Errorf("modal did not advance to the next memory: %q", mid.secretTitle)
	}

	tm, _ = tm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	end := tm.(Model)
	if end.mode != modeNormal {
		t.Errorf("walk did not finish, mode=%v", end.mode)
	}
	if end.scanFlagged != nil {
		t.Error("walk state not cleared after finishing")
	}
}

// esc abandons the whole batch, not just the memory being asked about.
func TestBatchWalkEscCancelsBatch(t *testing.T) {
	var tm tea.Model = walkModel(t, "block")
	tm, _ = tm.Update(tea.KeyMsg{Type: tea.KeyEsc})
	got := tm.(Model)
	if got.mode != modeNormal || got.scanFlagged != nil || got.scanAccepted != nil {
		t.Errorf("esc left batch state behind: mode=%v flagged=%v accepted=%v",
			got.mode, got.scanFlagged, got.scanAccepted)
	}
}

// block-strict offers no override, but skipping stays available: it promotes
// less rather than overriding anything, which is what strict wants.
func TestBatchWalkStrictSkipsButCannotInclude(t *testing.T) {
	var tm tea.Model = walkModel(t, "block-strict")
	before := len(tm.(Model).scanAccepted)

	tm, _ = tm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if got := tm.(Model); got.scanIdx != 0 || len(got.scanAccepted) != before {
		t.Errorf("block-strict accepted an override: idx=%d accepted=%d", got.scanIdx, len(got.scanAccepted))
	}
	out := ansi.Strip(tm.(Model).secretModal())
	if strings.Contains(out, "y include") {
		t.Errorf("strict modal offered an include action:\n%s", out)
	}
	if !strings.Contains(out, "n skip this") {
		t.Errorf("strict modal should still offer skip:\n%s", out)
	}

	tm, _ = tm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	if got := tm.(Model); got.scanIdx != 1 {
		t.Errorf("n did not advance under block-strict: idx=%d", got.scanIdx)
	}
}

// The modal names which memory is being judged and where the walk is.
func TestBatchWalkModalNamesMemory(t *testing.T) {
	m := walkModel(t, "block")
	out := ansi.Strip(m.secretModal())
	if !strings.Contains(out, "x") || !strings.Contains(out, "1 of 2") {
		t.Errorf("modal should name the memory and the position:\n%s", out)
	}
}

// A walk that accepts nothing must not promote — and must say so.
func TestBatchWalkAcceptingNothing(t *testing.T) {
	m := walkModel(t, "block")
	m.scanAccepted = nil // nothing was clean either
	var tm tea.Model = m
	tm, _ = tm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	tm, cmd := tm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	got := tm.(Model)
	if got.mode != modeNormal {
		t.Fatalf("walk did not finish, mode=%v", got.mode)
	}
	if cmd == nil {
		t.Fatal("expected a status command reporting the cancellation")
	}
	if got.scanSkipped != 0 || got.scanAccepted != nil {
		t.Errorf("walk state not reset: skipped=%d accepted=%v", got.scanSkipped, got.scanAccepted)
	}
}

// Marks are spent by a finished batch: leaving them set would silently aim the
// next promote at a set the user already acted on.
func TestMarksClearedAfterBatch(t *testing.T) {
	m := ready(t)
	m.marks = map[string]bool{"/p/a.md": true, "/p/b.md": true}
	m.batchItems = batchFixture()

	var tm tea.Model = m
	tm, _ = tm.Update(promoteFinishedMsg{pushed: true, count: 2})
	got := tm.(Model)
	if len(got.marks) != 0 || got.batchItems != nil {
		t.Errorf("marks/batch survived a finished promote: marks=%d items=%d", len(got.marks), len(got.batchItems))
	}
}

// A single promote must not clear marks it never consumed.
func TestSinglePromoteLeavesMarksAlone(t *testing.T) {
	m := ready(t)
	m.marks = map[string]bool{"/p/a.md": true}
	var tm tea.Model = m
	tm, _ = tm.Update(promoteFinishedMsg{pushed: true}) // count 0 = single
	if n := len(tm.(Model).marks); n != 1 {
		t.Errorf("single promote cleared marks: %d, want 1", n)
	}
}

// A filter typed after marking hides rows; the marks it hides must still be
// promoted, or the batch would quietly do less than the user marked.
func TestBatchIncludesFilterHiddenMarks(t *testing.T) {
	m := ready(t)
	hidden := m.memories[0]
	m.marks = map[string]bool{hidden.Path: true}

	// A filter that cannot match the marked memory.
	m.search.SetValue("zzzz-no-such-memory")
	m.rebuildRows()
	for _, r := range m.rows {
		if r.kind == rowMemory && r.item.Path == hidden.Path {
			t.Fatal("setup: the marked memory is still visible")
		}
	}

	tm, _ := m.actionPromoteBatch()
	got := tm.(Model)
	if len(got.batchItems) != 1 || got.batchItems[0].path != hidden.Path {
		t.Errorf("filter-hidden mark was dropped: %+v", got.batchItems)
	}
}

func TestPromoteNounAndSkippedSuffix(t *testing.T) {
	if got := promoteNoun(promoteFinishedMsg{count: 3}); got != "promoted 3 memories" {
		t.Errorf("batch noun = %q", got)
	}
	if got := promoteNoun(promoteFinishedMsg{count: 1}); got != "promoted" {
		t.Errorf("single noun = %q", got)
	}
	if got := skippedSuffix(0); got != "" {
		t.Errorf("no skips should add nothing, got %q", got)
	}
	if got := skippedSuffix(2); !strings.Contains(got, "2 skipped") {
		t.Errorf("skipped suffix = %q", got)
	}
}

// The batch scan must read EVERY memory, not just the first — a batch promote
// must not become a way to walk a credential past the guard behind a clean file.
// This drives the real scan command over real files rather than hand-building
// its result, which is the only way the "every memory" claim is actually tested.
func TestBatchScanCoversEveryMemory(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("---\nname: "+name+"\n---\n"+body+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	// The secret sits in the LAST memory: a scan that stops early reports clean.
	items := []batchItem{
		{path: write("one.md", "nothing to see"), title: "one", placement: "global"},
		{path: write("two.md", "still nothing"), title: "two", placement: "global"},
		{path: write("three.md", "key = AKIAIOSFODNN7EXAMPLE here"), title: "three", placement: "global"},
	}

	m := ready(t)
	msg, ok := m.batchScanCmd(items)().(batchScanFinishedMsg)
	if !ok {
		t.Fatal("batchScanCmd did not return a batchScanFinishedMsg")
	}
	if msg.err != nil {
		t.Fatalf("scan error: %v", msg.err)
	}
	if len(msg.flagged) != 1 {
		t.Fatalf("flagged %d memories, want 1 — the scan missed a later memory", len(msg.flagged))
	}
	if msg.flagged[0].item.title != "three" {
		t.Errorf("flagged %q, want the last memory", msg.flagged[0].item.title)
	}
	if len(msg.clean) != 2 {
		t.Errorf("clean = %d, want 2", len(msg.clean))
	}
}

// A memory that cannot be scanned stops the whole batch: promoting what we could
// read while silently skipping what we could not is exactly the hole the guard exists to close.
func TestBatchScanUnreadableStopsBatch(t *testing.T) {
	m := ready(t)
	msg := m.batchScanCmd([]batchItem{
		{path: filepath.Join(t.TempDir(), "missing.md"), title: "missing", placement: "global"},
	})().(batchScanFinishedMsg)
	if msg.err == nil {
		t.Fatal("an unreadable memory must fail the batch scan")
	}
	if !strings.Contains(msg.err.Error(), "missing") {
		t.Errorf("error should name the memory, got %q", msg.err)
	}
}

// Including a flagged memory is a real decision, so the result line must say it
// happened — a batch with an override must never read as a clean promote.
func TestBatchResultDisclosesOverrides(t *testing.T) {
	if got := overrodeSuffix(0); got != "" {
		t.Errorf("no overrides should add nothing, got %q", got)
	}
	if got := overrodeSuffix(1); !strings.Contains(got, "1 overridden") {
		t.Errorf("override suffix = %q", got)
	}
	if got := overrodeSuffix(3); !strings.Contains(got, "3 overridden") {
		t.Errorf("override suffix = %q", got)
	}
}

// The walk counts an inclusion as an override, so it can be disclosed later.
func TestBatchWalkCountsOverrides(t *testing.T) {
	var tm tea.Model = walkModel(t, "block")
	tm, _ = tm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if got := tm.(Model).scanOverrode; got != 1 {
		t.Errorf("scanOverrode = %d after including a flagged memory, want 1", got)
	}
}

// fakeStore makes team.IsInitialized() report true, so the promote entry points
// can be driven in a test without a real clone.
func fakeStore(t *testing.T) {
	t.Helper()
	base := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", base)
	if err := os.MkdirAll(filepath.Join(base, "engram", "team", ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
}

// Cancelling the scope picker must disarm the batch. A batch left armed is picked
// up by the NEXT promote — including a single-memory one, which would then promote
// memories the user never selected.
func TestScopeCancelDisarmsBatch(t *testing.T) {
	m := ready(t)
	m.batchItems = batchFixture()
	m.mode = modePromoteScope

	var tm tea.Model = m
	tm, _ = tm.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if n := len(tm.(Model).batchItems); n != 0 {
		t.Errorf("%d batch items survived the cancel", n)
	}
}

// Clearing marks clears the batch they built.
func TestEscClearingMarksDisarmsBatch(t *testing.T) {
	m := ready(t)
	m.marks = map[string]bool{"/p/a.md": true}
	m.batchItems = batchFixture()

	var tm tea.Model = m
	tm, _ = tm.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if n := len(tm.(Model).batchItems); n != 0 {
		t.Errorf("%d batch items survived clearing the marks", n)
	}
}

// The two promote modes are mutually exclusive by construction: each entry point
// clears the other's state, so the scope picker can never be ambiguous about
// which one it is serving.
func TestPromoteModesAreMutuallyExclusive(t *testing.T) {
	fakeStore(t)

	// Entering a batch clears the single-memory state.
	m := ready(t)
	m.marks = map[string]bool{m.memories[0].Path: true}
	m.promotePath, m.promoteTitle = "/stale/single.md", "stale"
	tm, _ := m.actionPromoteBatch()
	got := tm.(Model)
	if got.promotePath != "" || got.promoteTitle != "" {
		t.Errorf("batch entry left single state live: path=%q title=%q", got.promotePath, got.promoteTitle)
	}
	if len(got.batchItems) != 1 {
		t.Fatalf("batch not armed: %d items", len(got.batchItems))
	}

	// Entering a single promote clears a leftover batch.
	m2 := ready(t)
	m2.batchItems = batchFixture()
	tm2, _ := m2.actionPromote()
	if n := len(tm2.(Model).batchItems); n != 0 {
		t.Errorf("single promote inherited %d stale batch items", n)
	}
}

// A failed batch keeps its marks. PromoteBatch prepares everything before the
// first write, so a refusal leaves nothing applied — the user should be able to
// fix the cause and retry, not re-mark every memory from scratch.
func TestFailedBatchKeepsMarks(t *testing.T) {
	m := ready(t)
	m.marks = map[string]bool{"/p/a.md": true, "/p/b.md": true}
	m.batchItems = batchFixture()

	var tm tea.Model = m
	tm, _ = tm.Update(promoteFinishedMsg{count: 4, err: errors.New("would both promote as global/notes.md")})
	got := tm.(Model)
	if len(got.marks) != 2 {
		t.Errorf("marks = %d after a failed batch, want 2 (retry must not need re-marking)", len(got.marks))
	}
}

// A batch that only failed to PUSH did write and commit locally, so its marks are
// spent — re-promoting those memories would be a no-op.
func TestUnpushedBatchStillSpendsMarks(t *testing.T) {
	m := ready(t)
	m.marks = map[string]bool{"/p/a.md": true}
	var tm tea.Model = m
	tm, _ = tm.Update(promoteFinishedMsg{count: 2, pushed: false})
	if n := len(tm.(Model).marks); n != 0 {
		t.Errorf("marks = %d after a local commit, want 0", n)
	}
}
