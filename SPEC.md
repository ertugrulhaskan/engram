# engram — Specification

This document is the source of truth for what engram is, how it's built, and the
decisions behind it. It's written for contributors. For user-facing docs see
[README.md](README.md); for sequencing see [ROADMAP.md](ROADMAP.md).

---

## 1. Goal

A fast, open-source **terminal UI** for browsing, searching, and editing the
memories Claude Code keeps on disk — across all projects — and for **sharing**
the useful ones with a team over any git host.

Non-goal: replacing `CLAUDE.md`-in-a-repo. engram covers what that can't —
cross-project memories, personal-vs-team layering, and a real UI.

## 2. Principles

1. **Read-only by default.** Never modify a memory file unless the user
   explicitly edits or promotes it.
2. **Compatible with Claude Code.** Files stay readable by Claude. engram only
   ever *adds* optional frontmatter it understands; it never rewrites Claude's
   fields.
3. **No servers.** Sharing rides on plain git, so the project stays free to run
   and host-agnostic (GitHub / GitLab / Bitbucket / self-hosted).
4. **Single binary.** Ships as one downloadable file. Go makes this free.
5. **Small, layered code.** File logic and UI never mix.

## 3. Tech stack

| Concern            | Choice                                            |
|--------------------|---------------------------------------------------|
| Language           | Go 1.23+                                           |
| TUI framework      | Bubble Tea (Charm)                                |
| List / viewport    | Bubbles (Charm)                                   |
| Styling            | Lip Gloss (Charm)                                 |
| Markdown rendering | Glamour (Charm)                                   |
| Frontmatter        | `gopkg.in/yaml.v3`                                |
| Sharing transport  | git (shelled out), any host                       |
| License            | MIT                                               |

## 4. Where memories live

Claude Code stores memories per-project:

```
~/.claude/projects/<encoded-project-path>/memory/
    MEMORY.md          # human index of the folder
    <slug>.md          # one memory per file
```

`<encoded-project-path>` is the project's absolute path with a leading `-` and
both `/` and `.` replaced by `-`, e.g. `/Users/me/code/app` → `-Users-me-code-app`
and `/Users/me/code/engram.im` → `-Users-me-code-engram-im`.

> ⚠️ **Decoding is lossy.** A `-` in the encoded name may have been a `/`, a `.`,
> or a literal `-` (`acme-site`, `work-bigco`, `engram.im`), so the
> mapping back is ambiguous. engram decodes by walking the filesystem, matching
> each path component against a real folder on disk (with its dots flattened), so
> multi-separator names like `app.engram.im` resolve in full. When no real path
> resolves — e.g. the project was deleted — it falls back to a best-effort
> slash-joined path (`-Users-ghost-engram-im` → `/Users/ghost/engram/im`). Decoding
> is never guaranteed, but it always yields a usable project identity.

## 5. The two on-disk memory shapes

engram must parse **both**, because real installs contain both:

**A. YAML frontmatter** (the documented format):
```markdown
---
name: dev-server-already-running
description: the dev server is usually already up on :3000
metadata:
  type: project
---
Body in markdown…
```

**B. Plain markdown** (older / hand-written), with metadata in `MEMORY.md`:
```markdown
# User preferences
Body in markdown…
```
```markdown
<!-- MEMORY.md -->
- [User preferences](user-preferences.md) — highly detail-oriented on UI/design
```

### Parsing rules (precedence)

For each `*.md` in a memory dir (excluding `MEMORY.md`):

| Field         | Source order                                                        |
|---------------|---------------------------------------------------------------------|
| `Name`        | frontmatter `name` → filename without `.md`                         |
| `Title`       | first `# ` heading in body → `MEMORY.md` link title → titleized name |
| `Description` | frontmatter `description` → `MEMORY.md` hook → first body paragraph |
| `Type`        | frontmatter `metadata.type` → `unknown`                             |
| `Body`        | file content minus frontmatter                                      |

`MEMORY.md` index lines are parsed with:
`- [Title](file.md) — hook`  (em-dash, en-dash, or hyphen separators accepted).

## 6. Data model

```go
type Memory struct {
    Name        string   // slug
    Title       string   // human title
    Description string   // one-line hook
    Type        Type     // user | feedback | project | reference | unknown
    Body        string   // markdown body (no frontmatter)
    Raw         string   // full original file contents
    Path        string   // absolute path on disk
    Modified    time.Time // file modification time (drives recency + auto-reload)
    Project     Project
}

type Project struct {
    Name      string // friendly (basename of decoded path)
    Dir       string // decoded absolute project dir (best-effort)
    MemoryDir string // .../memory
    Remote    string // git remote URL — Phase 2, empty in Phase 1
}

// DocFile is a read-only instruction file surfaced in the /files source
// (Phase 1.5; the non-Claude files added in Phase 4 tier 1). No frontmatter — an
// assistant manages these, engram never hand-edits them.
type DocFile struct {
    Path, Title, Body string
    Kind              DocKind     // "rules" (the instruction files) | "index" (MEMORY.md)
    Provider          DocProvider // "claude" | "agents" | "gemini" | "copilot"
    Scope             string      // "global" or the project name
    ProjectName, ProjectDir, MemoryDir string
    Modified          time.Time
}
// Kind and Provider are deliberately separate dimensions: every instruction file
// is the same *kind* (rules) from a different ecosystem. Collapsing them would
// force a new DocKind per vendor as tier 1 grows.

// projectRuleFiles is the single table of per-project instruction files, in
// display order. DiscoverDocs, DocsSignature and docRank all read it, so adding
// a vendor is one row rather than four edits that can drift apart.
var projectRuleFiles = []ruleFile{
    {"CLAUDE.md",                            "CLAUDE.md",               ProviderClaude},
    {"AGENTS.md",                            "AGENTS.md",               ProviderAgents},
    {"GEMINI.md",                            "GEMINI.md",               ProviderGemini},
    {".github/copilot-instructions.md", "copilot-instructions.md", ProviderCopilot},
}

// globalRuleFiles is the matching table for the vendors' *global* instruction
// files — the ones that apply to every project and so live in the home dir,
// outside the ~/.claude tree the project walk is anchored to. Claude's own
// ~/.claude/CLAUDE.md is read separately, since it sits under claudeHome rather
// than home. Both docs scans walk this table too, for the same lockstep reason.
var globalRuleFiles = []ruleFile{
    {".codex/AGENTS.md",  "AGENTS.md",  ProviderAgents},
    {".gemini/GEMINI.md", "GEMINI.md",  ProviderGemini},
}
```

## 7. Sharing design (Phase 2)

> **Status:** implemented and merged to `main` — `init-team`, `promote`, `withdraw`,
> `pull` (with clean-update fast-forward), the secret-scan guard, a **sync anchor**
> (`syncedHash`) driving direction-aware state pills (`synced`/`behind`/`ahead`/
> `conflict`/`missing`, with `unknown` as the
> no-anchor fallback), the `global`/`project` scope chip, the `>resolve` **conflict-
> resolve** UX, pull reconciling global-scoped memories, and multi-select promote,
> including **promoting a whole type at once** — the type filter is the selection,
> and `a` marks it.
> Remaining: alias *coordination* — the remote-less alias fallback itself shipped
> 2026-08-29 (`>alias`).

The shared store is **one git repo** the whole team can read/write. engram keeps
a managed local clone and shells out to git for all sync.

### Interface & storage

- **Setup is a one-time CLI subcommand:** `engram init-team <git-url>`. It clones
  the team repo to `~/.config/engram/team/` (alongside the existing config), and
  if the repo is empty, scaffolds `global/`, `projects/`, and `MEMORY.md`.
- **Day-to-day, the team verbs live under the `>` command palette** (`ctrl+p` → `>`:
  `>promote`, `>pull`, `>withdraw`, `>resolve`, `>init <git-url>`, `>alias <name>`), a third prefix beside
  `/` sources and `@` assistants, so engram stays a no-arg TUI for normal use. `>init` mirrors
  the `engram init-team` subcommand, which remains for first-run/CLI setup.
- **No servers and no engram-level auth.** Access is whatever the git host grants
  on the repo; push/pull use the user's existing git credentials (SSH / credential
  helper). engram surfaces a clear error when credentials or the remote are missing.

### Project identity across machines

A teammate's local project path differs from yours, so **project-specific
memories are keyed by git remote URL**, not local path. engram reads the remote
with `git -C <project-dir> remote get-url origin`, then **normalizes** it to a
canonical `host/path` slug — lowercased, with the protocol, any `user@`, a
trailing `.git`, and a trailing `/` stripped — so `git@github.com:acme/app.git`,
`https://github.com/acme/app`, and `ssh://git@github.com/acme/app` all map to the
same `github.com/acme/app`. Monorepos sharing one remote share one bucket in Phase 2
(sub-keys are a later refinement).

Projects with no remote fall back to a **user-assigned alias**: `>alias <name>` on a
memory in the project stores it as `projectAliases` (memory dir → alias) in the config
— keyed by the memory dir because that is the project's stable identity, where the
project dir is decoded best-effort from Claude's folder name and can change when the
tree does. `team.NormalizeAlias` lowercases it — for the same reason `NormalizeRemote`
does, so two spellings can't become two buckets a case-folding filesystem then merges
— and admits exactly one path component that is safe on every platform the store
might be checked out on (no slash, leading or trailing dot, or Windows device name),
and `team.AliasKey` turns it into the key `alias/<name>`, so the store tree shows
which buckets came from an alias. The remote always wins, and the fallback is narrow
on purpose: `projectKey` (promote, batch promote) and `resolveTargets` (pull) consult
the alias only for `team.ErrNoRemote` — the directory exists and either no repository
encloses it or the repository has no `origin` — and for a directory that has since
vanished, whose alias was granted while it was there and had no remote. Never for any
other `ProjectKey` failure (an origin whose URL has no host/path, a repository git
won't read), which would otherwise let a stale alias override a real remote on a
hiccup. The session does not go stale on any of this: the 2s poll re-reads the whole config —
`projectAliases` and `secretScanAction` included, not just `scanRoots` — and a tick
carries the settings generation it was launched with, so a tick that read the file
*before* an in-session `>alias` is discarded rather than undoing it. (The theme is
deliberately excluded: colours changing under a poll tick is a surprise, and `1`–`3` and
`/settings` are both explicit.) The rule lives once, in `team.ResolveKey`, which promote
and pull both call;
`team.ClassifyRemote` names the answer git reached (found / none / gone / home /
reserved / unknown), and `ResolveKey` returns it beside the key, so `>alias` refuses by
state and the promote dialog explains a key-less project truthfully — suggesting
`>alias` for `RemoteNone` alone, not when git couldn't say and not on the home folder,
which refuses it. `RemoteUnknown` is deliberately the **zero value** of `RemoteState`,
for the reason `source.Caps`'s zero value grants nothing: a state nobody set is a state
nobody asked git about, and it must claim least — as `RemoteFound` (which asserts a key
exists) or `RemoteNone` (which invites `>alias`) it would let an unset field decide what
a dialog offers. `ProjectKey` classifies
a failure by git's exit status, not its message (2: no such remote; 128: not a
repository — or one git can't read, so git itself is asked which with `rev-parse
--git-dir`, under its own discovery rules rather than a hand-rolled `.git` walk). So
a project that later gains a remote promotes under its remote key from then on; what
was already promoted under the alias stays in that bucket, which pull reports as
skipped, until it is promoted again (pull reads one key per memory dir — a second
would double the tombstone pass's accounting — so migrating that bucket is a
follow-up). A project whose directory has since vanished keeps using its alias (the
alias was granted while the directory was there and had no remote; its going missing
creates none — this is what keying by memory dir is for), though no new alias can be
set on one. The map is validated once where it is read, by `team.CleanAliases`: values
normalized, a malformed name dropped, a name two memory dirs both claim dropped from
both — pull would treat one key in two memory dirs as one repo cloned twice, right for
a remote and wrong for two projects — so promote and pull agree by construction.
`>alias` refuses, saying why, on a project that has a remote (naming the key), on one
git can't answer for, on Claude's home-folder project, on a name another project already holds
(`team.SetAlias` — a holder whose memory dir is gone still holds the store bucket its
memories were shared under, so the name is freed deliberately via `/settings`, never
reclaimed), and on a config file that doesn't parse. That last rule is enforced in one
place, `config.Update`: it reads through `config.Read`, which tells absent from
unparseable (`ErrUnparseable`; an empty file counts as absent, there being nothing in it
to protect), so every writer — `>alias`, the theme keys, the seeded
settings file — refuses rather than replacing the user's settings with defaults, and
says so. Its counterpart is that engram must not be able to *create* such a file:
`config.Save` writes a temp file beside the target and renames it over, because
`os.WriteFile` truncates before it writes and this file is written on every `1`/`2`/`3`
keypress and every `>alias` — interrupted between the two, it would leave a half-object
that `Read` reports as `ErrUnparseable` and startup treats as fatal, and the repair that
message advertises (`/settings`) is inside the TUI that would no longer open. It does not
`fsync`: the failure being closed is a *process* death mid-write, against which the
rename is complete on its own, and buying the power-loss case as well would mean blocking
the event loop on every theme keypress, since `config.Update` is called inline from the
key handler. Four details the rename has to carry by hand, because `os.WriteFile` got them
free. A config the user made read-only is refused as `os.WriteFile` refused it: the target
is opened for writing first (no truncate) and the error returned, since a rename alone needs
only the directory. The temp file is opened through `os.OpenFile` at the mode the config will *end up*
with, not through `os.CreateTemp` (which hardcodes `0600`): the umask then narrows a
**new** config exactly as it always did — a user on `umask 077` keeps a private file —
and the temp is never wider than its destination, not even for the moment before a
corrective chmod. An **existing** config's own mode is then restored onto the temp
before the rename, because `os.WriteFile` applied its perm only at creation, so a mode
the user set survived every later write; without that, a `chmod 600` would be undone by
the next theme keypress, widening a file that holds the editor command, scan roots and
project aliases. (`Perm()` masks to the `0777` bits, so setuid/setgid/sticky are never
copied.) And a **symlinked** config is resolved first, so a `config.json` that stow or
chezmoi links into a dotfiles repo keeps receiving writes — `os.WriteFile` wrote
*through* the link, while a bare rename would replace it with a regular file and
silently strand the repo copy. `O_EXCL` is what makes the temp's predictable name safe:
a file or symlink pre-planted at that name fails the open and the loop moves on, so the
write can never be redirected through one. A *dangling* link is followed too, by hand —
`EvalSymlinks` fails outright when the target is missing, and falling back to the link's
own path would have the rename eat the link. That is the normal dotfiles bootstrap order,
the link made before the repo copy exists, and `os.WriteFile` handled it by opening
through the link and creating the target. The rename does need write permission on the *directory*, which
`os.WriteFile` did not once the file existed; a read-only config dir holding a writable
`config.json` now fails to save rather than saving non-atomically, which is the rarer
situation and the better failure. There is no read-only convenience wrapper: a
`config.Load` that answered an unreadable file with defaults was deleted once the last
writer moved off it, because that policy is the one every rule here argues against and
the short name kept inviting it back; the `/settings` reload applies the file it just read without saving it back,
and names any `projectAliases` entry `CleanAliases` had to ignore. The `alias/`
namespace is reserved at the key level: `NormalizeRemote` refuses a remote whose host
is literally `alias` (`ErrReservedHost`), because for a single-segment path the
collision is *exact* — `alias:acme` normalizes to `alias/acme`, which is
`AliasKey("acme")` byte for byte — so two unrelated projects would share one store
bucket and `applyPull`'s `byKey` would cross-place their memories into each other.
Accepting the overlap was tried and reverted: the argument for it, that `IsAliasKey`
only decides the promote dialog's "(your alias)" caption, holds for *multi-segment*
paths, where the remote lands in a subdirectory the alias bucket never reads — and it
was reached by checking only those. **A namespace claim has to be checked at its
shortest form, where the prefix is the whole key.** The refusal carries its own error
and its own `RemoteState` (`RemoteReserved`) so the dialogs say what happened: git
answered correctly, engram declined the answer, and the fix is renaming the ssh alias
— not the `RemoteUnknown` line, which would blame git. The *other* half of the key
gets the same treatment for the same reason: a trailing dot or space is trimmed from
the host **and from every path segment**, because the key becomes a directory path in
the store and NTFS strips both — so `github.com/acme/app.` and `github.com/acme/app`
would be two buckets on the author's machine and one on a teammate's Windows checkout,
reaching the identical cross-placement failure through the path instead of the host. A
path segment that is a Windows *device* name (`con`, `nul`, `lpt1`) is knowingly **not**
refused, unlike in `NormalizeAlias`: an alias is a name the user invents and can change
on request, a remote is not, so refusing one would strand a real repository over a
checkout platform its owner may never use. Trimming loses nothing; refusing would. The one segment trimming cannot help is `..`:
dropping it rewrites the path into another repository's bucket, which is what
`filepath.Clean` did at placement, so `NormalizeRemote` refuses it and names it; `.` is
dropped, as Clean dropped it; a segment that is only dots or spaces names no directory on
NTFS and is refused too. `placementPath` keeps a `..` check of its own, on the raw
segments, for a key read off a store file's frontmatter. Two witnesses decide "no repository":
`rev-parse --git-dir` exiting 128 *and* no `.git` entry up the tree — either seeing a
repository means git could not say, since a repository git refuses to read (dubious
ownership) exits 128 from both commands. What the alias does not solve is
coordination: two teammates must independently choose the same name to meet (§10).

**An alias never keys the home folder; a real remote does.** An alias is a name invented
for a project that has no identity of its own, and Claude's home-folder project — the
memories of sessions run outside any repository — is not a project in that sense. It is
**a state, not a flag beside one**: the home folder reaches `ProjectKey` exactly like any
remoteless project and comes back `ErrNoRemote`, so `ClassifyRemote` asks `team.IsHomeDir`
and answers `RemoteHome`, which falls past the alias branch of `ResolveKey` and which
`>alias` and the promote dialog each refuse by name. The earlier shape carried the fact
alongside the state — a `promoteAliasable` bool on the Model — and it was wrong in the way
a parallel field always is: three callers each paid their own `IsHomeDir` (a
`UserHomeDir` plus two `EvalSymlinks`, on the event loop, on top of the git spawn
`ResolveKey` already pays for), and a caller that set the state but forgot the bool got
the *most permissive* wording by default. One value now carries the whole answer.
A home directory that is itself a dotfiles repo keeps its remote key and promotes under
it, exactly as any other repository — it never reaches `RemoteHome`, because
`ClassifyRemote` answered `RemoteFound` first. A middle version of this refused the
remote too, on the reasoning that it would publish personal notes into a team bucket;
that was wrong twice over — the bucket is not a privacy boundary (a promote copies the
memory into the shared store whichever bucket it lands in, behind an explicit scope
dialog that names the key), and refusing it would strand memories already shared under
that key.

### Shared repo layout

```
team-memory/
    global/<slug>.md            # apply everywhere
    projects/<remote-slug>/<slug>.md   # apply to one project
    MEMORY.md
```

### engram-only frontmatter

Added to files engram manages; Claude Code ignores unknown keys:

```yaml
engram:
  id: 7f3a9c1e-…                    # stable UUID, assigned on first promote
  scope: team                       # personal | team
  project: github.com/acme/app      # normalized git remote, or "global"
  owner: you@acme.com
  syncedHash: 9f2a3c…               # digest of the shared content at last sync (the base)
```

`syncedHash` is the **sync anchor**: a short digest (`memory.ContentDigest`) of the
memory's *shared content* — Claude's frontmatter and body, with engram's own block
excluded so the anchor never hashes itself — recorded on every promote and pull. It is
the common base engram compares against to distinguish a clean fast-forward (`behind`)
from a real conflict, and to split `unknown` into a direction. It is a within-version
change-detection optimization, not a security primitive: a memory without it (shared
before this release) simply falls back to the direction-less `unknown`, and a digest that ever
fails to line up degrades to a conservative conflict — never a silent overwrite.

`id` is the **durable identity**: a memory keeps it across slug renames and
edits, so a renamed promotion *updates* its team copy instead of orphaning it
and creating a duplicate, and the same memory is matched across machines even if
a teammate's local filename differs. The slug (`<slug>.md`) is just the
filename. The id is assigned once, on the first promote.

### Operations

- **promote** *(`>promote`)* — copy the selected personal memory into the clone, set
  `engram.scope: team`, assign an `engram.id` if absent, commit, push. A modal picks the
  scope, defaulting to the current project, with a "global" option.

  **A batch is one act.** `space` marks memories; `promote` then acts on the marked
  set rather than the cursor row. The scope modal offers *their own projects* — each
  memory goes to its own key, and one whose project has no remote falls to `global`,
  which the modal states — or `global` for all. `team.PromoteBatch` stamps and stages
  every memory and then makes **one commit and one push**, because N separate commits
  is history noise for what the user performed once. Preparation is complete before
  the first write, so a batch that cannot go through (unsafe key, unreadable memory,
  or two memories claiming the same store path — compared case-folded, since such a
  pair is one file on macOS and Windows) leaves every local file untouched.
  Marks survive a filter: a memory hidden by a search still promotes, since dropping
  it would quietly promote less than was marked.

  **Every memory in a batch is scanned**, not just the first — a batch must not become
  a way to walk a credential past the guard. Flagged memories are decided one at a
  time: `y` includes, `n` skips (leaving that one personal), `esc` abandons the batch.
  Skipping stays available under `block-strict`, since it overrides nothing. The
  accepted memories still land as a single commit, and the toast reports both what
  was skipped and what was included past a finding — an override is a real decision
  and is never left unsaid.
- **withdraw** *(the reverse of promote; `>withdraw`, owner-only)* — remove the
  memory's copy from the store (matched by `engram.id`), record its id in a
  `.engram-withdrawn` **tombstone ledger**, and reset the local `engram.scope` to
  personal (keeping the id). It **pre-checks that the local reset can be written before
  touching the store** — if the frontmatter can't be safely rewritten, withdraw refuses
  up front and leaves the memory fully shared, rather than removing the store copy and
  then failing the reset (which would strand the memory as `! missing`). Commit + push
  both the removal and the tombstone. Only
  the `owner` (the promoter's git email) may withdraw — an **advisory guardrail**,
  not enforcement, since anyone with push access can bypass it. When either side of
  that comparison is unknown — the memory records no `owner`, or this machine has no
  `user.email` — the guard **fails open and says so**: the confirm dialog names which
  side is missing, because refusing to withdraw your own memory over a misconfigured
  git would be the worse failure. The two causes are reported separately, since one
  is the memory's history and the other is one config line away. On a teammate's next
  **pull**, the tombstone removes their local team-scoped copy too, so a withdrawal
  **propagates** — this is the one case where pull deletes a local file, and it
  deletes *only* a tombstoned `scope: team` copy that is no longer anywhere in the
  store **and still matches its sync anchor** (an unshared local edit, or a copy with
  no anchor to check against, is kept — never a silent loss of work). A personal file
  is never removed, and a tombstoned copy is **demoted to personal rather than deleted
  whenever pull cannot positively prove it is someone else's**: the owner's own copy on
  another machine (the owner keeping their memory, just un-shared), but equally a copy
  with no `owner` recorded, or one whose owner cannot be compared because this machine
  has no `user.email` set. Deletion is reserved for an owner that is recorded,
  comparable, and *different* — anything less fails toward keeping the file, matching
  the guard in **withdraw**, which fails open for the same reason. A stale `scope: team`
  stamp on a kept file surfaces as `! missing` and is recoverable; a deleted file is not.
  **Re-promoting clears the tombstone** (so a re-shared memory isn't deleted), and a
  memory still shared under another scope is kept. Named `withdraw`, not
  "unpromote"/"demote".

  **The ledger is append-only, and that is a decision, not a gap.** A tombstone may
  safely be dropped only once *every* clone has pulled past it, and a git store keeps
  no roster of clones and no record of who has synced — so the condition is not
  answerable with what engram has. Drop one early and a clone that never saw it simply
  never deletes its copy: the memory stays `scope: team` on that machine, still read
  by Claude there, surfacing only as a `! missing` badge. It does not return to the
  store — pull never writes to it — so the blast radius is one clone, not the team.
  But that is precisely the failure the ledger exists to prevent, and it matters most
  in the case that motivates withdrawing at all: a memory un-shared *because* it
  should not have been shared.

  The trade is lopsided, and measured rather than assumed. The ledger holds **one line
  per distinct id that is currently withdrawn**, not one per withdrawal — re-promoting
  clears the entry, so withdraw/re-promote cycles do not accumulate. Its ceiling is
  therefore the number of distinct memories ever shared to this store and left
  withdrawn: bounded by human authoring, not by anything a machine can run up. An
  entry is one 70-byte line and `readWithdrawn` runs **once per pull** (its result is a
  set, so the per-memory lookups are O(1)). Measured: 1,000 entries is 71 KB read in
  0.2 ms; 10,000 is 722 KB in 2.4 ms; 100,000 is 7.1 MB in 15 ms — under the network
  round-trip of the `git pull` that just ran. Keeping a stale tombstone costs bytes;
  dropping a live one costs correctness.

  The two alternatives were weighed and rejected. **Age-based expiry** accepts exactly
  the resurrection risk above for any clone dormant past the horizon, and the ledger
  format carries no timestamps — it would need either a format break for existing
  stores or a per-line `git log`. **Per-clone sync markers** would answer the safety
  condition, but they turn every `pull` into a `push` (a read that can now fail,
  conflict, and need credentials), make clones visible to one another, and let a clone
  that is deleted rather than deregistered block collection forever. Both buy real
  machinery against a cost measured in kilobytes.
- **pull** — `git pull` the clone, then place project team files where Claude reads
  them, remove any local team copy whose id was withdrawn upstream (see **withdraw**),
  and refresh the relevant `MEMORY.md`. For a memory already present locally, the
  **sync anchor** decides: only the store moved and the local is untouched → **fast-
  forward** (take the store copy); only the local moved → left as `ahead`; both moved
  (or no anchor) → left as a conflict, never overwritten. The summary counts new /
  updated / ahead / up-to-date / withdrawn / conflict / skipped. In the TUI the
  accounting comes **first**: `team.PullPlan` runs the identical walk with writes
  off (the fetch itself is safe — it only fast-forwards the store's cache repo),
  a confirm dialog shows the counts, and `team.PullApply` then applies that same
  walk without a second fetch — so the confirmed plan and the apply can never
  disagree. A zero-work plan skips the dialog. Pull also walks `global/`: an
  existing local copy of a global memory (matched by `engram.id`, tracking
  `engram.project: global`) gets the same anchor-driven decision, but a global
  memory with no local copy is never *placed* — it belongs to no project, so the
  store stays its home until the user promotes it into one, and it is not counted
  as skipped.
- **resolve** *(`>resolve`)* — reconcile a `conflict` / `unknown` (or, by hand, a
  `behind`) memory. engram writes the two versions' **shared content** (Claude frontmatter + body,
  engram block excluded) into a temp file bracketed by git-style markers
  (`<<<<<<< yours … ======= … >>>>>>> team`), opens `$EDITOR`, and on save writes the
  resolved content back — re-anchoring on the store version so "take theirs" reads as
  `synced` and a kept merge reads as `ahead`. A file still holding a marker line, or
  emptied, aborts with the memory untouched. *(Whole-content markers in the file, so a
  frontmatter-only divergence is surfaced too.)* The confirm before `$EDITOR` shows an
  **inline diff** of the two sides — see §8.4.
- **Sync is manual.** Personal memories never leave the machine unless promoted,
  and engram never auto-pulls. On launch it does a cheap check against the team
  repo and badges memories that have updates (a `[behind]` pill); files are only
  placed when you run `pull`.
- **Rejected push** (non-fast-forward) → engram runs `git pull --rebase` and
  retries once; if that conflicts, it hands off to the user.

### Secret-scan guard (promote)

Promoting pushes a memory into a shared git repo, where a leaked credential is
effectively permanent. So before a push, engram scans the memory
(`internal/secrets.Scan`, pure pattern + entropy analysis; `internal/team.ScanForSecrets` reads the
file) and applies a configured policy:

- `secretScanAction`: `block` (default — modal with the redacted findings and a
  `y` override) · `block-strict` (no override) · `warn` (footer note, promote) ·
  `off` (skip). Mechanism lives in `internal/secrets`/`internal/team`; the policy
  (which action) lives in the TUI. A scan error blocks (fails closed).
- `secretScanScope`: `secrets` (default — keys/tokens/private keys) · `secrets+pii`
  (also emails and card-like numbers; noisier).
- `scanRoots`: extra directories to look for projects in (§8.2).
- `projectAliases`: memory dir → alias, for projects with no git remote; set with
  `>alias <name>`, cleared with `>alias -` (the alias fallback under "Project identity
  across machines").

Findings are **always redacted** (a short prefix + mask); the raw secret is never
rendered or logged. For a provider key that prefix is a format marker (`AKIA`,
`ghp_`) and gives nothing away; for a value matched by entropy alone there is no
marker, so it is four characters of the value — kept because the finding has to be
locatable in the file, and four characters do not narrow a search for the rest. Three layers cover most real cases: **by value shape** (provider
key formats, JWTs, `scheme://user:pass@` URLs) regardless of the variable name;
**by name** — any identifier containing `secret`/`token`/`password`/`api_key`/
`access_key`/`private_key`/`client_secret` before a `=`/`:`, so framework env vars
(`REACT_APP_…`, `VITE_…`, `NEXT_PUBLIC_…`, `NUXT_…`) are caught whatever the prefix;
and **by entropy** (below), for the value neither of those can see. All three run
**twice**: once over physical lines, and once over *logical* ones,
with the line breaks that merely wrap a value removed — so a credential split by a
soft wrap, a YAML block scalar, or a backslash continuation is still seen whole. A
break counts as a wrap only when credential-alphabet runs meet across it and at
least one carries a digit or runs long, which is what stops ordinary prose from
fusing; a spanning match reports the line the value *starts* on. When nothing is
wrapped the second pass sees exactly what the first did and contributes nothing.

**The entropy layer** is what sees a blandly-named var holding a raw generated
value — no recognizable format, no secret-word in its name, so nothing the first
two layers read. It judges runs of at least 28 credential characters scoring 4.4
bits or more of Shannon entropy, and speaks only when the other layers are silent,
so a recognized key is never reported twice.

Its thresholds are **measured, not chosen**, and the two are not independent:
entropy is capped at log2(len), so 28 characters puts the ceiling at 4.81 — high
enough for a real key to clear 4.4, low enough that hex text (SHAs, UUIDs, ~4.0 at
best) never can. Raw entropy is not sufficient on its own: a filesystem path is one
unbroken run (`/` and `-` are both base64 characters) and a long one scores
4.45–4.62, above any bar a 28-character key must also clear. So a run that is
**separator-joined words** — five or more digit-free pieces of 3–14 characters — is
vetoed structurally rather than by threshold. `internal/secrets.TestEntropyCorpus`
re-runs the measurement against a real tree of memories (set `ENGRAM_CORPUS_ROOT`);
it is the number to reach for before retuning either constant.

**Still not exhaustive.** The entropy layer deliberately misses hex-encoded secrets
(they score like the identifiers they are indistinguishable from) and anything
under 28 characters, and the word veto rejects a genuine key about once in 12,000
at worst. It's a guard paired with the informed override, not a guarantee; treat the
override as a real decision, not a rubber stamp.

### Sync-status (shown as badges in the list)

Every memory has a state relative to the team repo:

`SyncStates` (`internal/team/status.go`) matches a local memory to the store **by
`engram.id`** (a memory can appear under two scopes, so it counts as synced if it matches
*any* store copy), then `relationOf` — the single direction rule shared with pull's
`decidePull` — reads the anchor to name the state:

| Pill         | Meaning                                        | Suggested action |
|--------------|------------------------------------------------|------------------|
| *(none)*     | personal — local only, intentionally private   | —                |
| `[synced]`   | shared content matches a store copy            | —                |
| `[behind]`   | local is at the base; the store advanced       | pull / resolve   |
| `[ahead]`    | local advanced; the store is still at the base | promote          |
| `[conflict]` | both advanced past the base                    | `>resolve`       |
| `[unknown]`  | differs, but **no anchor** to name a direction | `>resolve`       |
| `[missing]`  | `scope: team` but its id is in no store copy   | promote          |

The pill is the bracketed state word, outlined in the state's semantic color (bold on
the selected row). The display words follow the design spec's vocabulary; the Go enum
keeps its original spellings (`StateIncoming` reads `behind`, `StateDiffers` reads
`unknown`) as a stable non-UI API.

The preview backs the pill with a **sync strip**: a band under the title carrying the
state's plain sentence, the offered action chip (`[p pull]`, from the same
`offeredAction` source of truth as the status bar), and a direction gauge
(`you ▬▬▬ ← ▬▬▬ team`, moved side filled in the state color, conflict both) with an
honest timestamp. `edited`/`diverged` stamps come from the local file's mtime;
`store advanced` is the store file's last **git commit time**
(`team.StoreLastChange`, fetched lazily per selected row, cached until reload) —
omitted whenever git can't answer, never fabricated. `missing`/`unknown` carry
`not in store` / `no anchor` instead of a time.

`[unknown]` is the honest fallback for a memory shared before the anchor existed:
distinguishing behind from ahead needs the recorded base, so without it engram makes no
direction claim. A color-coded `global` (OK green) / `project` (Info blue) **scope chip**
sits beside the pill — per-theme semantic tokens, like the sync colors — tied to its
presence (no orphan chip).

### Collisions & conflicts

- **Pull never overwrites a change.** Only a provable fast-forward (local digest equals
  the recorded base) rewrites a local file; an `ahead` or `conflict` (or any anchor-less
  `unknown`) is left untouched. Matching is by `id`, not filename.
- **Resolving** (`>resolve`) brackets both versions' shared content with git-style markers in
  `$EDITOR` and re-anchors on save (see **resolve** under §7 Operations), and previews the
  two sides as an inline diff first (§8.4).

**Known limits.** The anchor is a 64-bit digest — ample for change detection, and a
collision only ever degrades to a conservative conflict, never a silent overwrite. A
global memory the user holds nowhere is never placed by pull — it has no project to
land in; the store is its home until the user promotes it into one.

## 8. Module layout

```
engram/
    main.go                  # entry point: discover memories + plans → launch TUI; --version/--help; init-team subcommand
    internal/
        memory/              # NO UI here
            memory.go        # types; Caps — what the TUI may offer on memories (everything)
            discover.go      # the shared projects walk (eachProject/allProjects) + scanRoots; decode paths, fs signature
            parse.go         # frontmatter + index parsing, fallbacks
            index.go         # MEMORY.md index upsert / remove / reconcile
            docs.go          # read-only instruction-file + MEMORY.md discovery/signature (the /files source); DocsCaps (zero: read-only)
            edit.go          # create / delete / open-in-$EDITOR
            frontmatter.go   # engram: block (EngramMeta incl. syncedHash) — lossless round-trip; ContentDigest / ShareContent
        plan/                # discover plan-mode plans under ~/.claude/plans; Caps — view + delete only
        config/              # settings under the XDG config dir; Read tells absent from unparseable, Update is the one write path (never over a file that didn't parse), Save is atomic (temp file + rename); Dir() base-path helper
        team/                # NO UI here — shared team store over git (Phase 2)
            team.go          # package doc + Dir() (managed clone path) + IsInitialized()
            remote.go        # NormalizeRemote: git remote URL → canonical host/path key
            identity.go      # ProjectKey (git remote → key; ErrNoRemote by exit status), ClassifyRemote, ResolveKey (the one remote-first / alias fallback rule)
            alias.go         # NormalizeAlias / AliasKey / IsAliasKey / CleanAliases: the alias fallback for a project with no remote (projects/alias/<name>/)
            init.go          # InitTeam: clone team repo, scaffold empty layout, commit, push (engram init-team)
            promote.go       # Promote a memory into the store (global/ or projects/<key>/), stamp the anchor, commit, push
            scan.go          # ScanForSecrets: read a file and run internal/secrets over it (IO kept out of the TUI)
            pull.go          # Pull project team memories; anchor-driven fast-forward vs conflict (decidePull)
            status.go        # SyncStates + relationOf: read-only direction-aware sync state (synced/behind/ahead/conflict/unknown/missing)
            storetime.go     # StoreLastChange: a store memory's last git commit time (the "store advanced" stamp)
            withdraw.go      # Withdraw: owner-only removal + .engram-withdrawn tombstone
            ledger.go        # .engram-withdrawn tombstone ledger: record / look up withdrawn ids
            resolve.go       # BeginConflictResolve (returns a ResolveSession: the merge file + the two sides) / FinishConflictResolve: git-style $EDITOR merge (>resolve)
        secrets/             # NO UI here — pure credential scanning for the promote guard
            scan.go          # Scan: curated regexes + entropy layer over content → redacted findings (Scope: secrets / secrets+pii)
        source/              # NO UI, NO IO — the per-source capability model (§8.3)
            source.go        # Caps{Edit, Create, Delete, Share}: what a source lets the user do; zero value grants nothing
        tui/                 # NO file logic here
            tui.go           # package doc + shared enums/consts (focus, mode, srcKind, groupMode, typeCycle); srcCaps wiring + caps() gate + readOnlyHint
            model.go         # Model type, New, Init, theme/setTheme, styleInputs
            update.go        # Update dispatcher + per-mode key handlers
            view.go          # View, header block (tabs + controls rows, rule), status bar
            offer.go         # offeredAction + state sentences/glyphs: sync state → what the UI advertises
            synctime.go      # lazy per-selection fetch of the store's last-change time (sync strip stamp)
            items.go         # Item/row types, memory/plan → Item mapping, grouping, row build
            palette.go       # command palette: types, candidates, rendering
            render.go        # list/preview/row rendering, drift banner, manual rounded-dialog frame (frameLines)
            dialog.go        # shared dialog anatomy: icon+title header, wrapped body, Bg2 footer action band
            confirms.go      # pre-action confirms: pull accounting, resolve inline diff, reconcile naming files
            help.go          # ? help overlay: keybinding cheat-sheet + about footer
            teamactions.go   # >promote / >pull / >withdraw / >resolve / >init dispatchers + git-missing guard
            alias.go         # >alias: actionAlias / clearAlias (persist projectAliases via config.Update, the one write path) + projectKey adapter over team.ResolveKey
            promote.go       # >promote: team scope picker modal + background promote command
            promotebatch.go  # >promote over a marked set: each memory's own project key, one commit
            marks.go         # the marked set: `a` toggles every memory in the list (the type filter is the selection)
            pull.go          # >pull: resolve project keys, plan (PullPlan) + apply (PullApply) off-thread
            withdraw.go      # >withdraw: owner-only confirm modal + background withdraw command
            resolve.go       # >resolve: build the git-marker temp file, open $EDITOR, finish on save
            diff.go          # the resolve confirm's line diff: alignDiff (LCS + context collapse, frame-independent, cached) then capDiff per frame (§8.4)
            secret.go        # secret-scan modal: scan before promote, show redacted findings + override
            style.go         # color/pad/clip text helpers, type labels, humanize
            paint.go         # full-surface background painting (paintLine/paintBlock) + the dialog scrim (dimFrame)
            editor.go        # open-in-$EDITOR command plumbing + open-settings-file
            assistant.go     # the @ registry: which assistants exist, how each is invoked, $PATH detection
            seed.go          # assistant handoff: launch cwd + add-dir choice, seed prompt, orphan detection
            status.go        # transient footer status: kinds, flash/auto-dismiss
            layout.go        # resize geometry, glamour renderer build, listRows
            navigation.go    # cursor move/page, selection, source switch, preview sync
            reload.go        # fs polling + post-mutation reload commands
            theme.go         # themes, colors, group coloring, semantic status colors
            overlay.go       # floating dialogs
```

> Phase 1 also browses **plan-mode plans** as a second source (read-only, grouped by
> recency), switchable via the command palette. The sharing design below (Phase 2)
> concerns memories only.

### 8.1 `@` assistant handoff (Phase 1.5)

The palette is one sectioned list — *Jump to* (every memory and plan, capped at
30), then *Sources*, *Team*, and *Assistant* — filtered as a whole by typing:
fuzzy over item titles and project names, prefix over command/verb names. The
prefixes `/`, `>`, and `@` survive as section scopes (`>` keeps its
`>init <git-url>` argument parsing); nothing is reachable only via a prefix.
Rows are single lines (section sigil + label, description right-aligned) and
section headers are render-time lines, so the cursor only ever addresses
candidate rows.

The palette's `@` prefix offers AI assistants, held in one `assistants` registry
(`assistant.go`) that `palItem.provider` keys into: `@Claude`, `@Gemini`, `@Codex`
and `@Copilot`. The section lists whichever CLIs are on `$PATH` — offering an action
that cannot run is the same mistake the status bar's offered-action rule avoids — and
falls back to listing all four when none is installed, so `@` is never an empty section
that explains nothing. The bare `@` key opens this list rather than launching a
provider: with several installed there is no single right answer, and choosing silently
would start a session the user never picked.

Selecting one launches an **interactive** session via the same `tea.ExecProcess`
suspend/resume handoff `editor.go` uses for `$EDITOR` — no headless flag, no
engram-side diff UI; the assistant's own permission prompts gate edits. Two properties
are required of every registry entry, and a CLI lacking either does not belong in the
table: it must accept a seed prompt **without** dropping to non-interactive mode (each
of the four also has a one-shot `-p`-style flag that would silently turn the handoff
into a single answer), and it must be able to reach a directory outside its cwd, since
the memories live under `~/.claude` while engram launches in the project dir. The
spellings differ — `--add-dir` for Claude, Codex and Copilot, `--include-directories`
for Gemini — so each entry builds its own argv through an `args(prompt, addDir)`
closure. `TestAssistantArgsCarryAddDir` fails a future entry that forgets.

The launch is seeded so the session starts with context, not blind: `buildSeedPrompt`
injects the current source, the project/memory dirs, a live `memory.IndexDrift`
snapshot, and a soft scope ("memory/plan files only; ask before editing"). When the
assistant reads an instruction file of its own (`GEMINI.md`, `AGENTS.md`,
`.github/copilot-instructions.md`), the prompt names it and says these are Claude
Code's files — otherwise a non-Claude session can mistake the memories it was opened
to maintain for its own rules. The cwd is the selected memory's **project dir** when it
resolves and exists (so the assistant reads the right instruction file and, for Claude,
recalls the right memories); the memory dir lives under `~/.claude`, outside the
project, so it's passed as the add-dir for edit access. When the project
dir can't be resolved on disk — a **renamed/moved folder**, or a key that can't be reversed
to a real path (a `.` in the folder name flattens to `-` ambiguously) — engram launches in
the **`~/.claude/projects`** root instead: inside `.claude`, narrow relative to `$HOME`, and
broad enough that relocating memories across project keys needs no extra trust prompt
(the add-dir is then redundant and omitted). Because that fallback can be a false positive,
the seed prompt's wording is non-committal — it asks the assistant to relocate files only if
they are genuinely misfiled. Every assistant CLI is an **optional** runtime dependency:
absent, the provider is left out of the palette list, and selecting it from the
none-installed fallback shows a hint naming where to get it. On exit engram reloads (and
resets the drift cache), and the status line names the assistant that ran — the label
travels on `assistantFinishedMsg` rather than on the `Model`, because the exit callback
fires long after the `Update` that launched the session, so a `Model` field would be state
kept alive only to bridge those two moments.

### 8.2 `/files` read-only source (Phase 1.5; the non-Claude files in Phase 4 tier 1)

A third source (`srcFiles`, alongside `srcMemories`/`srcPlans`) surfaces the instruction
files an assistant *manages* rather than the ones you author: the global
`~/.claude/CLAUDE.md`, each project's `CLAUDE.md`, `AGENTS.md`, `GEMINI.md` and
`.github/copilot-instructions.md` (each only when its decoded dir resolves on disk — the same
lossy-key limitation as §8.1), and each project's `MEMORY.md`. Within a scope they sort in
`projectRuleFiles` order with `MEMORY.md` last (`docRank`, which ranks off that same table),
and the list badge names each: `rules`, `agents`, `gemini`, `copilot`, `index`. The four
vendor badges share one colour — the colour says "someone else's rules", the label says whose,
so a new vendor costs a word rather than another hue in a deliberately small palette.

The vendors' **global** files are read as well, from the `globalRuleFiles` table:
`~/.codex/AGENTS.md` and `~/.gemini/GEMINI.md` join `~/.claude/CLAUDE.md` in the `global`
scope. `claudeLayout` puts them in reach by resolving the home dir — it walks up from the
projects root to `~/.claude` and up once more — so the tree stays redirectable to a temp
dir in tests. A global doc belongs to no project, so it carries empty
`ProjectName`/`ProjectDir`/`MemoryDir`, exactly as `~/.claude/CLAUDE.md` always has. The
TUI reads that as "no repo to open": `assistantContext` returns `claudeHome()` when both
dirs are empty, so the `@` handoff launches in `~/.claude` rather than in an unrelated project.

Two vendor limits are deliberate. Copilot has no home-dir equivalent — VS Code keeps
user-level instructions in profile settings, not a file at a fixed path — and Codex's
`CODEX_HOME` relocation and its `AGENTS.override.md` precedence are not honoured, so
engram surfaces the default file only.

The **path-scoped** rule directories are covered by a second table, `projectRuleDirs`:
Cursor's `.cursor/rules/*.mdc` (badged `cursor`) and Copilot's
`.github/instructions/*.instructions.md` (badged `copilot`, beside its fixed file). Both
scans enumerate through one shared `ruleDirFiles` — recursive *within* the rules dir
(Cursor documents organizing rules in folders) and suffix-strict (Cursor itself ignores a
plain `.md` there). These files carry frontmatter, so the preview splits it off
(`splitFrontmatter`, the same parse memories use) and surfaces the scoping as a one-line
`Detail` — "applies to <globs/applyTo>", or "always applied" — read line-wise because real
`.mdc` files hold YAML-invalid plain scalars (`globs: **/*.ts`). Deliberately out:
monorepo-nested `.cursor/rules` under arbitrary subdirectories (a repo-tree walk per 2s
poll tick — the scanRoots depth-1 argument), the legacy `.cursorrules` (absent from
current Cursor docs), and Cursor/Copilot user-level rules (app-internal storage, no file).
`memory.DiscoverDocs`/`DocsSignature` walk these (the
signature folds into `combinedSig`, so external and `@`-assistant edits — including to `CLAUDE.md`,
which lives outside the memory tree — trigger the poll reload). **`DocsSignature` must stay in
step with `DiscoverDocs`:** a file surfaced by one but missed by the other displays correctly
and then never refreshes, a staleness bug no rendering test catches. They are **view-only**: the
`e` and `d` keys return a hint to edit via an assistant (`@`) rather than launching the editor or the
delete-confirm modal, so the index and instruction files aren't hand-corrupted. Selecting a
doc still carries its `ProjectDir`/`MemoryDir`, so launching an assistant from `/files` opens in
the right place. `MEMORY.md` remains auto-maintained by the `R` reconcile / index-sync;
"read-only" only governs direct hand-editing.

**Discovery is Claude-anchored by default, and tier 1 inherits that.** The walk enumerates
`~/.claude/projects/*` and reaches a project's working dir only by decoding that key, so by
default these files are found only for projects Claude Code already knows about.

**`scanRoots` lifts that.** `config.ScanRoots` names extra directories; `scanRootProjects`
checks each root **and its immediate children**, keeping a directory only when it carries an
instruction file (`hasRuleFile`: a `projectRuleFiles` entry, or a non-empty
`projectRuleDirs` directory). `allProjects` unions the two sources — Claude's
walk first, then the scanned ones it has not already claimed — and both docs scans go
through it, which is what keeps them in lockstep now that two mechanisms feed one list.

Three properties worth stating, because each is a decision rather than an accident:

- **Depth 1.** `DocsSignature` re-runs the scan on every 2s poll tick, so a recursive walk
  would re-read a whole workspace tree tens of times a minute. Dotted directories are
  skipped outright.
- **Additive, not fallback.** Scan roots are walked even when `~/.claude/projects` is
  missing, so a user who has never run Claude Code still sees their instruction files.
- **No memory dir.** A scanned project has none, so it contributes instruction files only.
  Both docs scans guard this: `filepath.Join("", "MEMORY.md")` is the *relative* path
  `MEMORY.md`, which would read whatever sits in the process's working directory.

Note that the global files are the one part of tier 1 that is *not* Claude-anchored — they are
read straight from the home dir, so they appear even with no `~/.claude` and no `scanRoots`.
Per-project discovery still is anchored, so public copy must go on not implying standalone
multi-assistant support out of the box — `scanRoots` is opt-in.

**Rendering text engram does not own.** `scanRoots` and the vendor tables mean a preview
can hold a file from a repository the user has merely cloned, which makes that file's bytes
untrusted input *to a terminal*. A scanned file is not the only such source: a team
memory's frontmatter arrives from the shared store, and a git failure carries the
subprocess's own captured output, so both reach the UI as text nobody on this machine
wrote. Neither glamour nor lipgloss strips an escape sequence embedded in text handed to
it, so engram strips control characters itself, at the four chokepoints every rendered
string passes through: `clip` for one-line metadata (titles, paths, badges, a rule file's
"applies to …" line), `wrapPlain` for dialog bodies, `flashStatus` for the status bar, and
`sanitizeBody` for the markdown body. A dialog body and the status bar need their own
chokepoint rather than leaning on `clip`: both render short strings whole, and `clip`
only ever sees a string long enough to truncate.
They all run **before** the renderer — sanitizing glamour's *output* would destroy the ANSI
it legitimately emits — and a body keeps newlines and tabs, which markdown uses
structurally, while a one-line sink turns a newline into a space so a multi-line git error
does not run its lines together.
Stripping before measuring also keeps the width math honest, since an escape sequence
occupies no display columns but its bytes would otherwise be counted as if it did.
Bidirectional-override characters (U+202A–U+202E, the "Trojan Source" class) are
deliberately **not** stripped: they are legitimate in right-to-left text, and what they
enable is visual deception rather than terminal control — a different problem needing a
different answer than a blanket filter.


### 8.3 Per-source capability (Phase 4, item 4)

What the user may *do* to what a source shows is a fact the source's data package
declares, not a `srcKind` comparison the TUI repeats at each key. `internal/source`
holds one type, `Caps{Edit, Create, Delete, Share}`, and each data package declares its
own next to the code that must honour it: `memory.Caps` grants everything (memories are
engram's own domain — an edit keeps the promise that engram only ever adds frontmatter
keys it owns), `plan.Caps` grants delete only (plans are plan-mode output engram has no
business editing; deleting a stale one is housekeeping), and `memory.DocsCaps` is the
zero value (instruction files belong to the assistant that reads them, so engram cannot
promise an edit stays compatible with that tool — they are browsable only, with repairs
routed through the `@` handoff to whichever assistant owns the file, §8.1).

The TUI wires those into one table, `srcCaps`, indexed by `srcKind`, and every
capability decision goes through one gate, `caps()`: `e`/`n`/`d` check `Edit`/`Create`/
`Delete`, the four team dispatchers and the batch marks check `Share`, and the controls
row is *derived* from the same struct, so the bar can only advertise a key the handler
will honour. A denied `e`/`n`/`d` answers with the source's `readOnlyHint` when it has
one (files name their escape hatch) and is otherwise silent — the key is absent from the
controls row too, so there is nothing to explain.

The gate decides *whether*; the executors decide *what runs*, and they fail closed too —
on their own terms, a routine either exists for the kind or it doesn't. The delete confirm routes on the item's kind with no fall-through — a kind
without a routine is refused, not guessed (the branch it replaced would have run
`memory.Delete` on an instruction file) — and the new-memory prompt refuses any source
but memories, since `currentMemDir` would otherwise fall back to the first project's
memory dir and plant the file somewhere unrelated. Neither refusal is reachable through
the keys today; they exist so that granting a capability to a new source without wiring
its routine fails visibly. `esc` clears marks only where they are drawn: on a source
without `Share` the marks wait for the return to memories.

Two consequences are the point. The **zero value grants nothing**, so a new source is
read-only until a capability is explicitly granted — forgetting to declare one fails as
a missing feature, never as a silently granted write. And the wiring is stated once,
which is what `TestCapsMatrix` pins: the decided matrix restated as a test, so a row
drifting from the decision fails by name. The checks that remain on `srcKind` (type
filter, grouping, reconcile, the drift banner) are deliberately not capabilities: they
are about what memories *are* — typed, grouped by project, indexed — not what the user
may do to them.

The governing test for granting `Edit` to any future source is the one above: can
engram keep the promise "stay compatible with the tool that owns this file"? Tier-2
imports (server-side memories brought in by export or paste) become regular memories
once imported — the import is an explicit user action, after which the vendor no longer
owns the copy, so full capability applies.


### 8.4 The resolve inline diff (Phase 2 refinement)

`>resolve` writes a git-marker merge file and opens it in `$EDITOR` (§7). The confirm in
front of that now shows the two sides **diffed** rather than the raw marker block, so the
decision is made on what actually differs.

`BeginConflictResolve` returns the two sides it merged (`ResolveSession`) alongside the temp
file, and `internal/tui/diff.go` aligns them. **The merge file is never parsed back into
halves**, and that is the whole design point: once the two sides are bracketed by markers, a
side that itself contains a line of equals signs — an ordinary setext heading underline — is
indistinguishable from the divider, so any split is a guess. A first attempt did parse the
file, on the argument that the preview would then provably match what `$EDITOR` opens; the
review gate produced the counter-example (`Backups / ======= / keep 30 days` on both sides
split at the *content* line and showed half of "yours" as the store's text), which is also
what makes the claim false. Returning the sides from the one place that has them unambiguously
costs nothing extra either: `BeginConflictResolve` already extracted both to build the file.

The alignment is a longest-common-subsequence diff with the common prefix and suffix trimmed
first, which for a typical edit leaves a handful of lines to align. `maxDiffCells` bounds the
table: a pathological pair falls back to showing each side whole — true, since every line
differs somewhere, if less precise — rather than stalling the UI thread. Unchanged runs longer
than two lines of context collapse to one row naming how many lines they stand for — *longer*,
because a run of exactly one is kept as the line itself: the elision would occupy the same
screen row and show strictly less, so it buys nothing and hides content. (With two lines of
context, two changes five lines apart leave exactly one line outside both windows, so the case
is ordinary, not contrived.) The whole view is capped at twelve rows so a long conflict can't
outgrow the frame — fewer when
the frame is short, because the budget is the rows `View` actually paints (`headerRows +
panesH + footerRows`, one short of the terminal, since `reservedRows` leaves the last row
unwritten) minus the modal's own chrome. That chrome is **measured**, by wrapping the
dialog's two text blocks through the same helper the renderer uses, not counted by hand: the
mechanics line is 61 cells against a text column of `boxWidth()-4`, so under 80 columns it
takes a second row. Only the cap depends on the frame, so the alignment is computed once per conflict and a
resize re-caps it: a drag-resize is a stream of `WindowSizeMsg`, and re-running the LCS
table per event — 250k cells for two 500-line memories, on the event loop — to change one
integer is what made the dialog stutter. Sized against the terminal's height with a
hand-counted chrome instead, the modal came out a row too tall and lost its footer off
the bottom — the only place the
confirm states what enter does, which is what deriving the budget was meant to protect. The capped
tail is its own kind of row, not an "unchanged" elision: what it hides is the rest of the
diff, changes included — possibly an entire side — and a confirm dialog that called that
"unchanged" would be telling the user the hidden remainder agrees.

Two presentation decisions worth recording. It is a **unified** diff, not two columns:
`boxWidth` caps a dialog at 68 cells, so side-by-side would leave about 32 per side and wrap
ordinary memory prose to shreds. And the two sides are colored `Warn` and `Info` — the same
colors the row badges use for `ahead` and `behind` — so "mine" and "the store's" read with a
vocabulary the user already has, rather than introducing a third pair. When the shared content
is identical and only the anchor differs (a resolve is reachable on `unknown`), the dialog
says that instead of rendering an empty diff.

## 9. Distribution

- `go install` for Go users.
- Tagged releases build cross-platform binaries (GitHub Releases / equivalent).
- Homebrew tap from the first release.

### Website

- Domain: **engram.im**. The landing page is `www/index.html`, with assets split into
  subfolders: `www/css/` (Tailwind) and `www/js/` (behavior). Styled with **Tailwind
  CSS — stock theme plus one custom token block**: the `.term` terminal-mockup palette
  in `www/css/input.css` (dark = the TUI’s Midnight theme; light = its Paperback
  theme, both based on `internal/tui/theme.go`; Paperback's small semantic text has
  web-only accessible foreground overrides). Everything outside the terminal mockups
  stays stock.
  Compiled to `www/css/styles.css`. Input is `www/css/input.css`; rebuild with `npm run build:css`
  (see CONTRIBUTING "Landing page"). The built CSS is **committed**, so there is no
  deploy-time build. Page behavior lives in `www/js/main.js` (a plain classic deferred
  script, no modules/dependencies); the only inline script is a tiny pre-paint theme
  guard in `<head>` (kept inline to avoid a flash of the wrong theme). It supports
  light / dark / system themes and is keyboard-accessible; without JavaScript, dead
  controls are hidden and a direct mobile nav replaces the drawer. It stays in sync
  with the docs.
- Served via **Cloudflare Pages** (static hosting, project `engram-im`): no build step
  (CSS is prebuilt & committed), upload directory `www`, custom domain `engram.im`.
  Deploys are manual — `npx wrangler pages deploy www --project-name=engram-im
  --branch=main` (omit `--branch` and you get a preview deployment that reports success
  while production is untouched). **DNS is
  on Cloudflare**, not Namecheap: the whole zone moved on 2026-08-07 because Cloudflare
  cannot serve an apex from another provider's nameservers. `cruz.ns.cloudflare.com` and
  `dean.ns.cloudflare.com` are set as Custom DNS at Namecheap, which stays the registrar
  only, and the apex resolves to Cloudflare's proxy addresses rather than to
  `engram-im.pages.dev`. The cost of that move is that Namecheap's email forwarding for
  the domain stopped working, which is why mail to `@engram.im` is accepted and then
  dropped. Note Pages serves pages
  extensionless and 308-redirects `/x.html` → `/x`, so links, canonicals and the
  sitemap use `/privacy`, not `/privacy.html`. Google Analytics is loaded **only after cookie consent** (see the
  banner in `index.html` + `main.js`); the [Privacy Policy](../www/privacy.html) is
  `www/privacy.html`. `www/404.html` is the not-found page: without it Pages answers an
  unknown path with the home page and a 200, which is wrong for both a reader and a
  crawler. It carries `noindex` and is deliberately absent from the sitemap.
- **Response headers** come from `www/_headers`, which Pages reads at deploy time and
  never serves. It sets HSTS, `X-Frame-Options`, a `Permissions-Policy` denying every
  feature the site does not use, and a Content-Security-Policy with no `'unsafe-inline'`:
  the inline theme guards are allowed by SHA-256 hash instead. That couples the header
  file to the markup — editing an inline script invalidates its hash and breaks the theme
  in production only — so the recompute command lives in `_headers` beside the hashes.
  Headers set there override Cloudflare's own, so the two it already sends correctly
  (`X-Content-Type-Options`, `Referrer-Policy`) are restated rather than omitted, keeping
  the whole policy in the repo.
- **Search / AI-answer surface.** `www/robots.txt` disallows nothing and names the AI
  *answer* agents (`OAI-SearchBot`, `ChatGPT-User`, `Claude-SearchBot`, `Claude-User`,
  `PerplexityBot`) separately from the *training* crawlers (`GPTBot`, `ClaudeBot`,
  `Google-Extended`, `CCBot`) — they are distinct user-agents, and both classes are
  allowed deliberately for an MIT-licensed public tool. `www/sitemap.xml` lists the two
  pages with hand-maintained `<lastmod>`. `www/index.html` carries `SoftwareApplication`
  and `FAQPage` JSON-LD backing a visible `#faq` section. `www/llms.txt` is a plain-text
  summary for LLMs, kept as a low-cost hedge rather than a measured win (no major vendor
  consumes it today). All four restate page content, so they are on the docs-sync list in
  CLAUDE.md; the `FAQPage` answers must match the visible cards word-for-word.
- **Published (Phase 3 done).** The repo is public, `v0.2.0` through `v0.5.1` are released, and the
  site is **live at engram.im on Cloudflare Pages** with SSL. For any *future* release, the tag push,
  visibility, and deploy stay the maintainer's deliberate steps — don't perform them
  unprompted (see the Releasing rules in CLAUDE.md / CONTRIBUTING.md).

## 10. Open questions / future

- Promoting whole *types* at once (e.g. "all feedback") — multi-select promote itself has shipped.
- Monorepo sub-keys. Subprojects that share one git remote share one bucket under
  `projects/<key>/`; per-subdirectory keys are a later refinement.
- Alias coordination for remote-less projects. The alias fallback shipped (`>alias`,
  keys under `projects/alias/<name>/`), but two teammates must still independently
  choose the *same* alias for their memories to meet — a shared alias map committed
  in the team repo is the likely fix.
- Phase 4: other assistants, in two tiers (source list in ROADMAP Phase 4). Local
  instruction files first — files on disk, no API needed. Server-side memories
  (Claude.ai / ChatGPT / Gemini app) stay blocked on those products exposing
  programmatic access; likely export/import at first.
- **Native Windows: out of scope, decided 2026-09-06.** This is no longer an open
  question. Binaries cross-compile and ship, but native Windows has never been run and
  two Unix assumptions are known to break it: `decodeProjectPath` resolves from `/`
  (Windows encodings carry a drive letter and fail the leading-`-` check), and the
  editor chain falls back to `vi`, which stock Windows does not have. Fixing either is
  cheap; *verifying* the fix needs a real Windows machine, which the maintainer does not
  have and does not intend to obtain, so an unverifiable fix would only convert a known
  gap into an unknown one. WSL works, being a Linux environment, and is what the README
  recommends. The Windows archives keep shipping because cross-compiling costs nothing
  and they are labelled untested; that is not a support claim, and public copy stays
  macOS/Linux. Reopen only if someone with a Windows machine offers to verify.
