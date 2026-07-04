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

// DocFile is a read-only CLAUDE.md / MEMORY.md surfaced in the /files source
// (Phase 1.5). No frontmatter — Claude manages these, engram never hand-edits them.
type DocFile struct {
    Path, Title, Body string
    Kind              DocKind // "rules" (CLAUDE.md) | "index" (MEMORY.md)
    Scope             string  // "global" or the project name
    ProjectName, ProjectDir, MemoryDir string
    Modified          time.Time
}
```

## 7. Sharing design (Phase 2)

> **Status:** implemented and merged to `main` — `init-team`, `promote`, `withdraw`,
> `pull` (with clean-update fast-forward), the secret-scan guard, a **sync anchor**
> (`syncedHash`) driving direction-aware badges (`✓`/`↓`/`↑`/`↕`/`!`, with `●` as the
> no-anchor fallback), the `global`/`project` scope chip, and the `>resolve` **conflict-
> resolve** UX. Remaining: auto-pull for global-scoped memories (today taken via `>resolve`),
> multi-select promote, and the remote-less alias fallback.

The shared store is **one git repo** the whole team can read/write. engram keeps
a managed local clone and shells out to git for all sync.

### Interface & storage

- **Setup is a one-time CLI subcommand:** `engram init-team <git-url>`. It clones
  the team repo to `~/.config/engram/team/` (alongside the existing config), and
  if the repo is empty, scaffolds `global/`, `projects/`, and `MEMORY.md`.
- **Day-to-day, the team verbs live under the `>` command palette** (`ctrl+p` → `>`:
  `>promote`, `>pull`, `>withdraw`, `>resolve`, `>init <git-url>`), a third prefix beside
  `/` sources and `@Claude`, so engram stays a no-arg TUI for normal use. `>init` mirrors
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
(sub-keys are a later refinement). Projects with no remote fall back to a
user-assigned alias.

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
the common base engram compares against to distinguish a clean fast-forward (`↓`) from a
real conflict (`↕`), and to split `●` differs into a direction. It is a within-version
change-detection optimization, not a security primitive: a memory without it (shared
before this release) simply falls back to the direction-less `●`, and a digest that ever
fails to line up degrades to a conservative conflict — never a silent overwrite.

`id` is the **durable identity**: a memory keeps it across slug renames and
edits, so a renamed promotion *updates* its team copy instead of orphaning it
and creating a duplicate, and the same memory is matched across machines even if
a teammate's local filename differs. The slug (`<slug>.md`) is just the
filename. The id is assigned once, on the first promote.

### Operations

- **promote** *(`>promote`)* — copy the selected personal memory into the clone, set
  `engram.scope: team`, assign an `engram.id` if absent, commit, push. A modal picks the
  scope, defaulting to the current project, with a "global" option. *(Single-select;
  multi-select promote is a later refinement.)*
- **withdraw** *(the reverse of promote; `>withdraw`, owner-only)* — remove the
  memory's copy from the store (matched by `engram.id`), record its id in a
  `.engram-withdrawn` **tombstone ledger**, and reset the local `engram.scope` to
  personal (keeping the id). Commit + push both the removal and the tombstone. Only
  the `owner` (the promoter's git email) may withdraw — an **advisory guardrail**,
  not enforcement, since anyone with push access can bypass it. On a teammate's next
  **pull**, the tombstone removes their local team-scoped copy too, so a withdrawal
  **propagates** — this is the one case where pull deletes a local file, and it
  deletes *only* a tombstoned `scope: team` copy that is no longer anywhere in the
  store **and still matches its sync anchor** (an unshared local edit, or a copy with
  no anchor to check against, is kept — never a silent loss of work). A personal file
  is never removed, and the **owner's own copy on another machine is demoted to
  personal, not deleted** (it is the owner keeping their memory, just un-shared).
  **Re-promoting clears the tombstone** (so a re-shared memory isn't deleted), and a
  memory still shared under another scope is kept. Named `withdraw`, not
  "unpromote"/"demote". *(Ledger entries are append-only; garbage-collecting old
  tombstones once every clone has synced is a later refinement.)*
- **pull** — `git pull` the clone, then place project team files where Claude reads
  them, remove any local team copy whose id was withdrawn upstream (see **withdraw**),
  and refresh the relevant `MEMORY.md`. For a memory already present locally, the
  **sync anchor** decides: only the store moved and the local is untouched → **fast-
  forward** (take the store copy); only the local moved → left as `↑ ahead`; both moved
  (or no anchor) → left as a conflict, never overwritten. The summary counts new /
  updated / ahead / up-to-date / withdrawn / conflict / skipped. *(Pull walks
  `projects/` only; a local copy of a global memory is updated via `resolve`.)*
- **resolve** *(`>resolve`)* — reconcile a `↕ conflict` / `● differs` / incoming-global
  memory. engram writes the two versions' **shared content** (Claude frontmatter + body,
  engram block excluded) into a temp file bracketed by git-style markers
  (`<<<<<<< yours … ======= … >>>>>>> team`), opens `$EDITOR`, and on save writes the
  resolved content back — re-anchoring on the store version so "take theirs" reads as
  `✓ synced` and a kept merge reads as `↑ ahead`. A file still holding a marker line, or
  emptied, aborts with the memory untouched. *(Whole-content markers, not a line-level
  diff, so a frontmatter-only divergence is surfaced too.)*
- **Sync is manual.** Personal memories never leave the machine unless promoted,
  and engram never auto-pulls. On launch it does a cheap check against the team
  repo and badges memories that have updates (a `↓ incoming` pill); files are only
  placed when you run `pull`.
- **Rejected push** (non-fast-forward) → engram runs `git pull --rebase` and
  retries once; if that conflicts, it hands off to the user.

### Secret-scan guard (promote)

Promoting pushes a memory into a shared git repo, where a leaked credential is
effectively permanent. So before a push, engram scans the memory
(`internal/secrets.Scan`, pure regexes; `internal/team.ScanForSecrets` reads the
file) and applies a configured policy:

- `secretScanAction`: `block` (default — modal with the redacted findings and a
  `y` override) · `block-strict` (no override) · `warn` (footer note, promote) ·
  `off` (skip). Mechanism lives in `internal/secrets`/`internal/team`; the policy
  (which action) lives in the TUI. A scan error blocks (fails closed).
- `secretScanScope`: `secrets` (default — keys/tokens/private keys) · `secrets+pii`
  (also emails and card-like numbers; noisier).

Findings are **always redacted** (a format prefix + mask); the raw secret is never
rendered or logged. Two layers cover most real cases: **by value shape** (provider
key formats, JWTs, `scheme://user:pass@` URLs) regardless of the variable name, and
**by name** — any identifier containing `secret`/`token`/`password`/`api_key`/
`access_key`/`private_key`/`client_secret` before a `=`/`:`, so framework env vars
(`REACT_APP_…`, `VITE_…`, `NEXT_PUBLIC_…`, `NUXT_…`) are caught whatever the prefix.
**The rule set is curated, not exhaustive** — a *blandly*-named var (a bare `*_KEY`
with no secret-word) holding a raw high-entropy blob (no recognizable format) is
matched by neither layer, and a secret split across lines is missed. It's a guard
paired with the informed override, not a guarantee; treat the override as a real
decision, not a rubber stamp.

### Sync-status (shown as badges in the list)

Every memory has a state relative to the team repo:

`SyncStates` (`internal/team/status.go`) matches a local memory to the store **by
`engram.id`** (a memory can appear under two scopes, so it counts as synced if it matches
*any* store copy), then `relationOf` — the single direction rule shared with pull's
`decidePull` — reads the anchor to name the state:

| Badge        | Meaning                                        | Suggested action |
|--------------|------------------------------------------------|------------------|
| *(none)*     | personal — local only, intentionally private   | —                |
| `✓ synced`   | shared content matches a store copy            | —                |
| `↓ incoming` | local is at the base; the store advanced       | pull / resolve   |
| `↑ ahead`    | local advanced; the store is still at the base | promote          |
| `↕ conflict` | both advanced past the base                    | `>resolve`       |
| `● differs`  | differs, but **no anchor** to name a direction | `>resolve`       |
| `! missing`  | `scope: team` but its id is in no store copy   | promote          |

`● differs` is the honest fallback for a memory shared before the anchor existed:
distinguishing incoming from ahead needs the recorded base, so without it engram makes no
direction claim. A color-coded `global` (teal) / `project` (azure) **scope chip** sits
beside the pill — fixed across themes like the sync colors — tied to its presence (no
orphan chip).

### Collisions & conflicts

- **Pull never overwrites a change.** Only a provable fast-forward (local digest equals
  the recorded base) rewrites a local file; a `↑ ahead` or `↕ conflict` (or any anchor-
  less differ) is left untouched. Matching is by `id`, not filename.
- **Resolving** (`>resolve`) brackets both versions' shared content with git-style markers in
  `$EDITOR` and re-anchors on save (see **resolve** under §7 Operations). An inline diff
  view is a later refinement.

**Known limits.** The anchor is a 64-bit digest — ample for change detection, and a
collision only ever degrades to a conservative conflict, never a silent overwrite. Global-
scoped memories aren't auto-placed by pull, so an incoming global update is taken via `>resolve`.

## 8. Module layout

```
engram/
    main.go                  # entry point: discover memories + plans → launch TUI; --version/--help; init-team subcommand
    internal/
        memory/              # NO UI here
            memory.go        # types
            discover.go      # walk projects, decode paths, fs signature
            parse.go         # frontmatter + index parsing, fallbacks
            index.go         # MEMORY.md index upsert / remove / reconcile
            docs.go          # read-only CLAUDE.md/MEMORY.md discovery + signature (the /files source)
            edit.go          # create / delete / open-in-$EDITOR
            frontmatter.go   # engram: block (EngramMeta incl. syncedHash) — lossless round-trip; ContentDigest / ShareContent
        plan/                # discover plan-mode plans under ~/.claude/plans (a second read-only source)
        config/              # load/save theme + editor under the XDG config dir; Dir() base-path helper
        team/                # NO UI here — shared team store over git (Phase 2)
            team.go          # package doc + Dir() (managed clone path) + IsInitialized()
            remote.go        # NormalizeRemote: git remote URL → canonical host/path key
            identity.go      # ProjectKey: resolve a project's git remote to its team key
            init.go          # InitTeam: clone team repo, scaffold empty layout, commit, push (engram init-team)
            promote.go       # Promote a memory into the store (global/ or projects/<key>/), stamp the anchor, commit, push
            scan.go          # ScanForSecrets: read a file and run internal/secrets over it (IO kept out of the TUI)
            pull.go          # Pull project team memories; anchor-driven fast-forward vs conflict (decidePull)
            status.go        # SyncStates + relationOf: read-only direction-aware sync state (✓/↓/↑/↕/●/!)
            withdraw.go      # Withdraw: owner-only removal + .engram-withdrawn tombstone
            ledger.go        # .engram-withdrawn tombstone ledger: record / look up withdrawn ids
            resolve.go       # BeginConflictResolve / FinishConflictResolve: git-style $EDITOR merge (>resolve)
        secrets/             # NO UI here — pure credential scanning for the promote guard
            scan.go          # Scan: curated regexes over content → redacted findings (Scope: secrets / secrets+pii)
        tui/                 # NO file logic here
            tui.go           # package doc + shared enums/consts (focus, mode, srcKind, groupMode, typeCycle)
            model.go         # Model type, New, Init, theme/setTheme, styleInputs
            update.go        # Update dispatcher + per-mode key handlers
            view.go          # View, top/bottom bars, drift warning, status styling
            items.go         # Item/row types, memory/plan → Item mapping, grouping, row build
            palette.go       # command palette: types, candidates, rendering
            render.go        # list/preview/row rendering and the manual rounded-dialog frame (frameLines)
            help.go          # ? help overlay: keybinding cheat-sheet + about footer
            teamactions.go   # >promote / >pull / >withdraw / >resolve / >init dispatchers + git-missing guard
            promote.go       # >promote: team scope picker modal + background promote command
            pull.go          # >pull: resolve project keys + pull team memories off-thread
            withdraw.go      # >withdraw: owner-only confirm modal + background withdraw command
            resolve.go       # >resolve: build the git-marker temp file, open $EDITOR, finish on save
            secret.go        # secret-scan modal: scan before promote, show redacted findings + override
            style.go         # color/pad/clip text helpers, type labels, humanize
            editor.go        # open-in-$EDITOR command plumbing + open-settings-file
            claude.go        # @Claude assistant: launch interactive Claude Code, seed prompt, context/orphan detection
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

### 8.1 `@Claude` assistant (Phase 1.5)

The empty palette is a guide: two `palPrefix` rows — `/` (commands) and `@`
(assistant) — each with a description. Selecting one seeds its prefix into the
input (equivalent to typing it), so `/` lists the source/settings commands and
`@` lists the assistant providers below.

The palette's `@` prefix offers AI providers (today only `@Claude`; the
`palProvider` registry and `palItem.provider` field keep room for others). Selecting
`@Claude` launches an **interactive** Claude Code session via the same
`tea.ExecProcess` suspend/resume handoff `editor.go` uses for `$EDITOR` — no `-p`
(headless), no engram-side diff UI; Claude Code's own permission prompts gate edits.

The launch is seeded so the session starts with context, not blind: `buildSeedPrompt`
injects the current source, the project/memory dirs, a live `memory.IndexDrift`
snapshot, and a soft scope ("memory/plan files only; ask before editing"). The cwd is
the selected memory's **project dir** when it resolves and exists (so Claude reads the
right `CLAUDE.md` and recalls the right memories); the memory dir lives under `~/.claude`,
outside the project, so it's granted with `--add-dir` for edit access. When the project
dir can't be resolved on disk — a **renamed/moved folder**, or a key that can't be reversed
to a real path (a `.` in the folder name flattens to `-` ambiguously) — engram launches in
the **`~/.claude/projects`** root instead: inside `.claude`, narrow relative to `$HOME`, and
broad enough that relocating memories across project keys needs no extra trust prompt
(`--add-dir` is then redundant and omitted). Because that fallback can be a false positive,
the seed prompt's wording is non-committal — it asks Claude to relocate files only if they
are genuinely misfiled. `claude` is a new **optional** runtime dependency: absent, the
action shows a hint and does nothing. On exit engram reloads (and resets the drift cache)
so changes appear immediately.

### 8.2 `/files` read-only source (Phase 1.5)

A third source (`srcFiles`, alongside `srcMemories`/`srcPlans`) surfaces the files Claude
*manages* rather than the ones you author: the global `~/.claude/CLAUDE.md`, each project's
`CLAUDE.md` (only when its decoded dir resolves on disk — the same lossy-key limitation as
§8.1), and each project's `MEMORY.md`. `memory.DiscoverDocs`/`DocsSignature` walk these (the
signature folds into `combinedSig`, so external/`@Claude` edits — including to `CLAUDE.md`,
which lives outside the memory tree — trigger the poll reload). They are **view-only**: the
`e` and `d` keys return a hint to edit via `@Claude` rather than launching the editor or the
delete-confirm modal, so the index and instruction files aren't hand-corrupted. Selecting a
doc still carries its `ProjectDir`/`MemoryDir`, so launching `@Claude` from `/files` opens in
the right place. `MEMORY.md` remains auto-maintained by the `R` reconcile / index-sync;
"read-only" only governs direct hand-editing.

## 9. Distribution

- `go install` for Go users.
- Tagged releases build cross-platform binaries (GitHub Releases / equivalent).
- Homebrew tap from the first release.

### Website

- Domain: **engram.im**. The landing page is `www/index.html`, with assets split into
  subfolders: `www/css/` (Tailwind) and `www/js/` (behavior). Styled with **Tailwind
  CSS (stock theme only — no custom colors/values/breakpoints)** compiled to
  `www/css/styles.css`. Input is `www/css/input.css`; rebuild with `npm run build:css`
  (see CONTRIBUTING "Landing page"). The built CSS is **committed**, so there is no
  deploy-time build. Page behavior lives in `www/js/main.js` (a plain classic deferred
  script, no modules/dependencies); the only inline script is a tiny pre-paint theme
  guard in `<head>` (kept inline to avoid a flash of the wrong theme). It supports
  light / dark / system themes and is keyboard-accessible; it stays in sync with the docs.
- Served via **Cloudflare Pages** (free tier, builds directly from the private repo —
  no GitHub Pro required): build command **empty** (CSS is prebuilt & committed), output
  directory `www`, custom domain `engram.im`.
- **Publishing is deferred.** Gate go-live on the v0.2.0 release being public so the
  install command and the "available" badge are true. Do not create the Cloudflare
  project, change DNS, or otherwise publish without explicit sign-off (see the
  Releasing rules in CLAUDE.md / CONTRIBUTING.md).

## 10. Open questions / future

- Inline diff view for `>resolve` (it currently opens both versions with git-style
  markers in `$EDITOR`; conflict resolution itself shipped in Phase 2).
- Promoting whole *types* at once (e.g. "all feedback"); multi-select promote.
- Phase 4: ingesting Claude.ai / ChatGPT / Gemini memories — blocked on those
  products exposing programmatic access; likely export/import at first.
