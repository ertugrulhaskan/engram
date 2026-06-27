# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

> Releases are versioned with SemVer (`v0.1.0`, …). The "Phase 1 / 1.5 / 2"
> labels in [ROADMAP.md](ROADMAP.md) are development milestones, not versions.

## [Unreleased]

**Phase 2 (team sharing over git) — core complete.** `init-team`, `promote`, and
`pull` are in place; sync-status badges and conflict-resolution UX are the remaining
pieces (see **Known gaps**). See [ROADMAP.md](ROADMAP.md) and [SPEC.md](SPEC.md) §7.

### Added
- **`engram init-team <git-url>`** — one-time setup subcommand that clones the
  shared team repo to `~/.config/engram/team/` and, when the repo is empty,
  scaffolds `global/`, `projects/`, and `MEMORY.md`, then commits and pushes.
  A failed push is non-fatal (the local commit is kept, with a retry hint), and
  git's own output — auth prompts, progress, errors — is shown directly.
- **Promote to team (`p`)** — in the TUI, promote the selected memory to the shared
  store: a scope dialog picks **this project** or **global**, then engram stamps the
  memory with an `engram:` frontmatter block (a durable id, scope, project, owner —
  preserving Claude's own keys), writes the copy under `global/` or
  `projects/<key>/`, and commits + pushes. A filename collision with a *different*
  memory is refused rather than overwritten.
- **Pull from team (`P`)** — fetch the team store and copy project-scoped team
  memories into their matching local projects (matched by git remote), refreshing
  each `MEMORY.md`. Pull never overwrites a differing local file (that's a conflict
  to resolve), no-ops identical ones, and skips projects with no local match.
  Global-scoped team memories stay in the store (browse / promote-on-demand).
- Internal: `internal/team` gains `ProjectKey` (resolve a project's git remote to its
  canonical key) and `Promote`; `internal/memory` gains a lossless `engram:`
  frontmatter round-trip (`ReadEngram`/`WriteEngram`, preserving Claude's keys) and
  a UUID helper. `NormalizeRemote` and the `config.Dir()` helper landed earlier.

### Changed
- **Landing page (`www/`) rebuilt** with Tailwind CSS (stock theme only) compiled to a
  committed `www/css/styles.css` via `npm run build:css`. Consolidated to a shorter layout
  with an interactive (auto-advancing, keyboard-accessible) terminal demo, a dedicated
  command-palette section, light / dark / system themes, and accessibility passes
  (ARIA tabs, focus-visible rings, `prefers-reduced-motion`). Build tooling
  (`package.json`, `www/css/input.css`) added; `node_modules/` is gitignored.
- **Landing-page assets split into subfolders** — `www/css/` (Tailwind input + built
  output) and `www/js/main.js` (page behavior as a plain classic deferred script, no
  modules or dependencies). Only a tiny pre-paint theme guard stays inline in `<head>`;
  the copy buttons are wired via `addEventListener` instead of inline `onclick`.

### Known gaps
- **Sync-status badges** (`[+] new`, `[team ✓]`, `[team ●]`, `[team ↓]`, `[team ⚠]`)
  are not yet rendered in the list — promote/pull work, but a memory's team state
  isn't shown at a glance yet.
- **Conflict-resolution UX** for `[team ⚠]` is pending: `pull` already *detects* and
  protects conflicts (never overwrites a differing local file), but the guided
  open-both-in-`$EDITOR` resolve flow isn't built.
- **Promote is single-select**; multi-select promote is pending.
- **No alias fallback** for projects without a git remote (they can't yet be keyed).
- No public release or Homebrew tap published yet — the git tags are local and
  publishing stays deferred until Phase 2 ships.

## [0.1.2] - 2026-06-25

A keybinding help overlay and a refreshed dialog style.

### Added
- **`?` — keybinding help overlay.** A floating cheat-sheet listing every key,
  grouped for readability, with an about footer (`version · engram.im · MIT`).
  Any key closes it; `?` is also shown in the bottom hint bar.

### Changed
- Restyle the floating dialogs (command palette, help, new-memory, delete
  confirm) as a flat rounded outline — smooth corners on the terminal
  background instead of a filled panel — with the selected/target row bleeding
  edge-to-edge to the border. The command-palette input now fills the dialog
  width.

### Fixed
- Dialog inputs no longer overflow the frame border: the command-palette and
  new-memory fields each reserved one cell too few for the text cursor.

## [0.1.1] - 2026-06-25

Memory-list polish.

### Changed
- Bold the selected row's title so the highlighted row stands out clearly, even
  on themes whose selection background sits close to the base background.
- Size the type-badge column to the widest badge currently listed (still capped
  at `[reference]`) instead of a fixed width, so short badges like `[user]` no
  longer leave a wide gap before the title in type-filtered and `/files` views.

## [0.1.0] - 2026-06-24

First release. Local memory **and** plan browsing (Phase 1), plus assisted
memory maintenance — `@Claude` and a read-only `/files` source (Phase 1.5). The
tag is local; publishing the release artifacts is deferred until Phase 2 (see
"Known gaps").

### Added
- Discover memories across all `~/.claude/projects/*/memory/` folders.
- Parse both on-disk memory shapes: YAML frontmatter, and plain markdown whose
  metadata comes from the project's `MEMORY.md` index.
- Two-pane TUI: searchable memory list + markdown preview rendered with Glamour.
- Filter / search memories with `/`.
- Edit the selected memory in `$EDITOR` (`e`), with reload on return.
- Create a new memory (`n`) from a title prompt, seeded with a frontmatter
  template, then open it in `$EDITOR`.
- Delete the selected memory (`d`) with a `y`/`n` confirmation.
- Cycle a type filter (`t`): all → user → feedback → project → reference →
  unknown.
- Group the memory list by project, with a colored header per group showing the
  project name and memory count `(N)`.
- Show a type badge on each memory (`[user]`, `[feedback]`, `[project]`,
  `[reference]`, `[other]`) so its kind is visible at a glance.
- Color-code type badges (user=blue, feedback=orange, project=green,
  reference=purple, other=gray) and add typography: colored group headers,
  indented rows, a `❯` cursor on the selected row, and dimmed descriptions.
- Toggle grouping between by-project and by-type with `g`.
- Keep the project's `MEMORY.md` index in sync: `new`/`delete`/`edit` upsert and
  remove the corresponding index bullets, and `R` reconciles a folder's index
  against its files (drops dangling bullets, adds missing ones).
- Discover and browse plan-mode plans alongside memories — a multi-source
  browser with a command palette (`ctrl+p`) and floating dialogs.
- Group the plans list by recency (Today / This week / Older) with the same
  colored headers, counts, and row layout as the memory list.
- Live theme switching (`1`–`5`) with a themed multi-pane layout.
- Persist the chosen theme and `$EDITOR` under the XDG config dir
  (`~/.config/engram/`).
- Auto-reload the browser when the memory files change on disk, detected via a
  lightweight filesystem signature.
- `engram --version` / `--help` report the build version and usage.
- **`/files` source** — a third, read-only browser (palette: `/files`) for the
  files Claude *manages* rather than the ones you hand-write: the global
  `~/.claude/CLAUDE.md`, each project's `CLAUDE.md` (when its directory resolves
  on disk), and each project's `MEMORY.md` index. They're view-only — `e`/`d`
  surface a hint to edit via `@Claude` instead — and changes made externally (or
  by `@Claude`) are picked up by the poll.
- **`@Claude`** in the command palette — type `@` in the palette (`ctrl+p`) to
  launch an interactive [Claude Code](https://claude.com/claude-code) session,
  seeded with the selected project's memory/plan health (index drift, locations,
  and — when a project folder was renamed — the orphaned-memory situation). It
  repairs what the `R` reconcile can't (malformed frontmatter, broken
  `[[links]]`, stranded memories) and can create, rewrite, merge, or reorganize
  memories/plans on request. engram suspends during the session (the same handoff
  as `$EDITOR`) and reloads on exit. Requires the `claude` CLI on `PATH`; when
  it's missing the action shows a one-line hint instead of failing.
- **Command palette guide rows** — the palette (`ctrl+p`) opens to two guide
  rows, **`/`** (commands) and **`@`** (assistant), each with a short
  description, instead of immediately listing every command. Typing `/` reveals
  `/memory`, `/plans`, `/files`, `/settings`; typing `@` reveals `@Claude`.
  Selecting a guide row seeds its prefix, so it doubles as a shortcut.
- **Severity-colored footer messages** — transient footer messages are
  color-coded: warnings and deletions render as white on red, cancellations as
  dark brown on emerald.
- Landing page for **engram.im** (`www/index.html`) — at the time, a single self-contained
  HTML page in the Classic Dark theme (later rebuilt with a Tailwind build step + light/dark/
  system themes; see [Unreleased]).
  Intended to be served via Cloudflare Pages from `www/`; publishing is deferred
  (see SPEC §9).
- Project `.mcp.json` registering the `context7` and `sequential-thinking` MCP
  servers so Claude Code (not just VSCode) can use them; the context7 key is read
  from the `${CONTEXT7_API_KEY}` environment variable, so no secret is committed.

### Changed
- The "index out of sync" warning names its cause — how many files were added
  without a `MEMORY.md` index line, and/or how many index entries point to a
  deleted/renamed file — instead of just flagging that drift exists.
- Internal: split the ~1.9k-line `internal/tui/tui.go` into focused same-package
  files (`model`, `update`, `view`, `items`, `palette`, `render`, `style`,
  `editor`, `status`, `layout`, `navigation`, `reload`); no behavior change.

### Fixed
- Project group names that contain dots (e.g. `engram.im`, `acme.dev`, or a
  domain-style folder like `app.engram.im`) now display in full. Claude flattens
  `/`, `.`, and `-` all to `-` when encoding a project folder, so decoding rebuilt
  `engram.im` as `engram/im` — showing the group as just `im`. Decoding now
  reconstructs the real path by matching folder names on disk, recovering dotted
  and multi-separator names. Affects both the memory and `/files` sources.

### Known gaps
- No public release or Homebrew tap published yet — the git tag is local, and the
  release tooling (GoReleaser + CI) is in place, but publishing is deferred until
  Phase 2.
- Team sharing over git (promote / pull, sync-status badges) is the next phase.

[Unreleased]: https://github.com/ertugrulhaskan/engram/compare/v0.1.2...HEAD
[0.1.2]: https://github.com/ertugrulhaskan/engram/compare/v0.1.1...v0.1.2
[0.1.1]: https://github.com/ertugrulhaskan/engram/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/ertugrulhaskan/engram/releases/tag/v0.1.0
