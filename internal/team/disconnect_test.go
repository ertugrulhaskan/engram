package team

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ertugrulhaskan/engram/internal/memory"
)

func idOf(t *testing.T, path string) string {
	t.Helper()
	m, _, err := memory.ReadEngram(string(must(os.ReadFile(path))))
	if err != nil {
		t.Fatal(err)
	}
	return m.ID
}

// Disconnect keeps every shared memory on the machine as a personal one, with
// its id, leaves personal memories byte for byte, removes only engram's clone,
// and leaves the remote's copies where they are.
func TestDisconnectKeepsTeamMemoriesAsPersonal(t *testing.T) {
	root, bare, writeMem := batchStore(t)
	shared := writeMem("shared.md", "shared body")
	personal := writeMem("personal.md", "personal body")
	if _, err := Promote(shared, "github.com/acme/app"); err != nil {
		t.Fatal(err)
	}
	id := idOf(t, shared)
	before := string(must(os.ReadFile(personal)))

	kept, err := Disconnect([]string{shared, personal})
	if err != nil {
		t.Fatal(err)
	}
	if kept != 1 {
		t.Errorf("kept = %d, want 1", kept)
	}
	if got := scopeOf(t, shared); got != "personal" {
		t.Errorf("the shared memory reads scope=%q after disconnect, want personal", got)
	}
	if got := idOf(t, shared); got != id {
		t.Errorf("the id changed from %q to %q; a later promote must re-share under the same id", id, got)
	}
	if !strings.Contains(string(must(os.ReadFile(shared))), "shared body") {
		t.Error("the shared memory lost its body")
	}
	if got := string(must(os.ReadFile(personal))); got != before {
		t.Errorf("the personal memory was rewritten:\n%s", got)
	}
	if IsInitialized() {
		t.Error("the store clone is still there")
	}
	if !remoteHas(t, root, bare, filepath.Join("projects", "github.com", "acme", "app", "shared.md")) {
		t.Error("disconnecting removed the memory from the remote")
	}
}

// Work the remote hasn't received (here, an offline promote's commit) must not
// vanish with the clone: Disconnect refuses and changes nothing.
func TestDisconnectRefusesUnpushedCommits(t *testing.T) {
	_, bare, writeMem := batchStore(t)
	shared := writeMem("shared.md", "body")
	restore := offline(t, bare)
	if pushed, err := Promote(shared, "github.com/acme/app"); err != nil || pushed {
		restore()
		t.Fatalf("offline Promote = (%v, %v), want a kept commit", pushed, err)
	}
	restore()
	_, err := Disconnect([]string{shared})
	if err == nil || !strings.Contains(err.Error(), "hasn't received") {
		t.Fatalf("Disconnect = %v, want a refusal naming unpushed commits", err)
	}
	if !IsInitialized() {
		t.Error("a refused disconnect removed the store")
	}
	if got := scopeOf(t, shared); got != "team" {
		t.Errorf("a refused disconnect changed the memory's scope to %q", got)
	}
}

func TestDisconnectRefusesUncommittedChanges(t *testing.T) {
	_, _, writeMem := batchStore(t)
	shared := writeMem("shared.md", "body")
	if _, err := Promote(shared, "github.com/acme/app"); err != nil {
		t.Fatal(err)
	}
	store, _ := Dir()
	writeStore(t, store, "stray.md", "half-finished\n")
	if _, err := Disconnect([]string{shared}); err == nil || !strings.Contains(err.Error(), "uncommitted") {
		t.Fatalf("Disconnect = %v, want a refusal naming uncommitted changes", err)
	}
	if !IsInitialized() || scopeOf(t, shared) != "team" {
		t.Error("a refused disconnect changed something")
	}
}

// A memory whose frontmatter can't be read stops the disconnect before any
// memory is touched, rather than leaving half of them personal.
func TestDisconnectChecksEveryMemoryBeforeWriting(t *testing.T) {
	_, _, writeMem := batchStore(t)
	shared := writeMem("shared.md", "body")
	if _, err := Promote(shared, "github.com/acme/app"); err != nil {
		t.Fatal(err)
	}
	broken := filepath.Join(filepath.Dir(shared), "broken.md")
	// An engram key that is there but isn't a block: it may be a shared memory
	// with a damaged block, so it is not guessed to be personal.
	if err := os.WriteFile(broken, []byte("---\nengram: oops\n---\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Disconnect([]string{shared, broken}); err == nil {
		t.Fatal("want an error naming the unreadable memory")
	}
	if scopeOf(t, shared) != "team" || !IsInitialized() {
		t.Error("the disconnect wrote before it had checked every memory")
	}
}

// Re-pointing is disconnect, then init: Remote names the store, then nothing,
// then the new one.
func TestDisconnectThenInitRepoints(t *testing.T) {
	root, bare, _ := batchStore(t)
	if got := Remote(); got != "file://"+bare {
		t.Fatalf("Remote() = %q, want file://%s", got, bare)
	}
	if _, err := Disconnect(nil); err != nil {
		t.Fatal(err)
	}
	if got := Remote(); got != "" {
		t.Errorf("Remote() after disconnect = %q, want empty", got)
	}
	next := filepath.Join(root, "next.git")
	gitT(t, "", "init", "--bare", next)
	if err := InitTeam("file://" + next); err != nil {
		t.Fatalf("InitTeam after disconnect: %v", err)
	}
	if got := Remote(); got != "file://"+next {
		t.Errorf("Remote() after re-pointing = %q, want file://%s", got, next)
	}
	if _, err := Disconnect(nil); err != nil {
		t.Fatal(err)
	}
	if _, err := Disconnect(nil); !errors.Is(err, ErrNoStore) {
		t.Errorf("Disconnect with no store = %v, want ErrNoStore", err)
	}
}

// The URL shown on screen never carries credentials.
func TestDisplayURLDropsCredentials(t *testing.T) {
	cases := map[string]string{
		"https://alice:tok3n@github.com/acme/store.git":         "https://github.com/acme/store.git",
		"https://alice@github.com/acme/store.git":               "https://github.com/acme/store.git",
		"https://gitlab.example.com/r.git?private_token=x#frag": "https://gitlab.example.com/r.git",
		"https://host.example.com/r.git?":                       "https://host.example.com/r.git",
		"git@github.com:acme/store.git":                         "git@github.com:acme/store.git",
		"file:///tmp/remote.git":                                "file:///tmp/remote.git",
	}
	for in, want := range cases {
		if got := displayURL(in); got != want {
			t.Errorf("displayURL(%q) = %q, want %q", in, got, want)
		}
	}
}

// A clone whose first push at init failed has no upstream. Disconnect must
// still judge it: here the scaffold commit never reached the remote, so it is
// refused with the folder named, not with "can't tell".
func TestDisconnectJudgesAStoreWithNoUpstream(t *testing.T) {
	root := t.TempDir()
	hermeticGitEnv(t, root)
	bare := filepath.Join(root, "remote.git")
	gitT(t, "", "init", "--bare", bare)
	if err := os.WriteFile(filepath.Join(bare, "hooks", "pre-receive"), []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	_ = InitTeam("file://" + bare) // the clone works, the push is refused
	if !IsInitialized() {
		t.Fatal("setup: want a clone left behind by the refused push")
	}
	store, _ := Dir()
	_, err := Disconnect(nil)
	if err == nil || !strings.Contains(err.Error(), "hasn't received") || !strings.Contains(err.Error(), store) {
		t.Fatalf("Disconnect = %v, want a refusal naming the unpushed commit and %s", err, store)
	}
}

// After a disconnect, nothing is left where the store was, so init can run.
func TestDisconnectLeavesNoFolderBehind(t *testing.T) {
	_, _, _ = batchStore(t)
	store, _ := Dir()
	if _, err := Disconnect(nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(store); !os.IsNotExist(err) {
		t.Errorf("the store folder is still there (%v)", err)
	}
	matches, _ := filepath.Glob(store + ".disconnected-*")
	if len(matches) != 0 {
		t.Errorf("the moved-aside copy was not deleted: %v", matches)
	}
}

// A clone with no commit yet (an init that failed before its first commit) has
// nothing to push, so it can be disconnected rather than refused with
// "can't tell".
func TestDisconnectAStoreWithNoCommits(t *testing.T) {
	root := t.TempDir()
	hermeticGitEnv(t, root)
	store, err := Dir()
	if err != nil {
		t.Fatal(err)
	}
	gitT(t, "", "init", "-q", store)
	if !IsInitialized() {
		t.Fatal("setup: want a store clone")
	}
	if _, err := Disconnect(nil); err != nil {
		t.Errorf("Disconnect on a store with no commits = %v, want it to succeed", err)
	}
}

// Commits on a branch HEAD no longer points at are still pending: with HEAD on
// an empty orphan branch, the clone's other branch holds unpushed work, and a
// disconnect must refuse rather than delete it.
func TestDisconnectCountsEveryLocalBranch(t *testing.T) {
	_, bare, writeMem := batchStore(t)
	shared := writeMem("shared.md", "body")
	restore := offline(t, bare)
	if _, err := Promote(shared, "github.com/acme/app"); err != nil {
		restore()
		t.Fatal(err)
	}
	restore()
	store, _ := Dir()
	gitT(t, store, "switch", "-q", "--orphan", "elsewhere") // empties the tree; the commit stays on the old branch
	if _, err := Disconnect(nil); err == nil || !strings.Contains(err.Error(), "hasn't received") {
		t.Fatalf("Disconnect = %v, want a refusal for the unpushed commit on the other branch", err)
	}
}

// A disconnect asked for while another team action holds the store is refused
// with nothing changed, rather than moving the clone out from under it.
func TestDisconnectRefusesWhileAStoreActionRuns(t *testing.T) {
	_, _, writeMem := batchStore(t)
	shared := writeMem("shared.md", "body")
	if _, err := Promote(shared, "github.com/acme/app"); err != nil {
		t.Fatal(err)
	}
	storeMu.Lock()
	_, err := Disconnect([]string{shared})
	storeMu.Unlock()
	if !errors.Is(err, ErrStoreBusy) {
		t.Fatalf("Disconnect while the store is busy = %v, want ErrStoreBusy", err)
	}
	if !IsInitialized() || scopeOf(t, shared) != "team" {
		t.Error("a refused disconnect changed something")
	}
}

// Every store action the TUI runs in the background waits for the lock before
// it does anything, so none of them can run while a disconnect holds it. Each
// is called with arguments it would reject at once, so one that skipped the
// lock returns inside the window.
func TestStoreActionsWaitForTheStoreLock(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	calls := map[string]func(){
		"PromoteBatch": func() { _, _ = PromoteBatch([]PromoteItem{{Path: "/nonexistent.md"}}) },
		"Withdraw":     func() { _, _ = Withdraw("/nonexistent.md") },
		"Pull":         func() { _, _ = Pull(nil) },
		"PullPlan":     func() { _, _ = PullPlan(nil) },
		"PullApply":    func() { _, _ = PullApply(nil) },
	}
	storeMu.Lock()
	done := make(chan string, len(calls))
	for name, call := range calls {
		go func() { call(); done <- name }()
	}
	select {
	case name := <-done:
		storeMu.Unlock()
		t.Fatalf("%s ran while the store lock was held", name)
	case <-time.After(300 * time.Millisecond):
	}
	storeMu.Unlock()
	for range calls {
		<-done
	}
}

// Disconnect is handed every memory, so a file deleted since it was listed is
// skipped, and so is one whose frontmatter can't be parsed at all (it never
// read as shared); the team memory beside them is still kept as personal.
func TestDisconnectSkipsDeletedAndUnparseableMemories(t *testing.T) {
	_, _, writeMem := batchStore(t)
	shared := writeMem("shared.md", "body")
	if _, err := Promote(shared, "github.com/acme/app"); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Dir(shared)
	garbled := filepath.Join(dir, "garbled.md")
	if err := os.WriteFile(garbled, []byte("---\nname: [unclosed\n---\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	before := string(must(os.ReadFile(garbled)))
	kept, err := Disconnect([]string{filepath.Join(dir, "gone.md"), garbled, shared})
	if err != nil {
		t.Fatalf("Disconnect = %v, want the deleted and unparseable files skipped", err)
	}
	if kept != 1 || scopeOf(t, shared) != "personal" {
		t.Errorf("kept = %d, scope = %q; want the team memory kept as personal", kept, scopeOf(t, shared))
	}
	if got := string(must(os.ReadFile(garbled))); got != before {
		t.Errorf("the unparseable memory was rewritten:\n%s", got)
	}
}

// FinishConflictResolve runs on the TUI's event loop, so it must not wait on the
// store lock: a background push holding it would freeze the screen.
func TestFinishConflictResolveDoesNotWaitForTheStoreLock(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	storeMu.Lock()
	defer storeMu.Unlock()
	done := make(chan struct{})
	go func() { _, _ = FinishConflictResolve("/nonexistent.md", "/nonexistent.tmp"); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("FinishConflictResolve waited for the store lock")
	}
}

// A shared memory whose YAML broke somewhere else in its frontmatter can't be
// read, but its engram line is still there: it may be team, so the disconnect
// stops and names it rather than leave it claiming a store that is gone.
func TestDisconnectRefusesAnUnparseableMemoryWithAnEngramLine(t *testing.T) {
	_, _, writeMem := batchStore(t)
	shared := writeMem("shared.md", "body")
	if _, err := Promote(shared, "github.com/acme/app"); err != nil {
		t.Fatal(err)
	}
	damaged := filepath.Join(filepath.Dir(shared), "damaged.md")
	if err := os.WriteFile(damaged, []byte("---\nname: [unclosed\nengram:\n  id: x\n  scope: team\n---\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Disconnect([]string{shared, damaged}); err == nil || !strings.Contains(err.Error(), "damaged.md") {
		t.Fatalf("Disconnect = %v, want a refusal naming damaged.md", err)
	}
	if !IsInitialized() || scopeOf(t, shared) != "team" {
		t.Error("a refused disconnect changed something")
	}
}

// A path listed twice is one memory: it is demoted once and counted once.
func TestDisconnectCountsARepeatedPathOnce(t *testing.T) {
	_, _, writeMem := batchStore(t)
	shared := writeMem("shared.md", "body")
	if _, err := Promote(shared, "github.com/acme/app"); err != nil {
		t.Fatal(err)
	}
	kept, err := Disconnect([]string{shared, shared})
	if err != nil {
		t.Fatal(err)
	}
	if kept != 1 {
		t.Errorf("kept = %d for one memory listed twice, want 1", kept)
	}
}
