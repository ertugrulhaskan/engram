# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- **`/files` source** — a third, read-only browser (palette: `/files`) for the files
  Claude *manages* rather than the ones you hand-write: the global `~/.claude/CLAUDE.md`,
  each project's `CLAUDE.md` (when its directory resolves on disk), and each project's
  `MEMORY.md` index. They're view-only — `e`/`d` surface a hint to edit via `@Claude`
  instead — and changes made externally (or by `@Claude`) are picked up by the poll.
- **`@Claude`** in the command palette — type `@` in the palette (`ctrl+p`) to launch
  an interactive [Claude Code](https://claude.com/claude-code) session, seeded with the
  selected project's memory/plan health (index drift, locations, and — when a project
  folder was renamed — the orphaned-memory situation). It repairs what the `R` reconcile
  can't (malformed frontmatter, broken `[[links]]`, stranded memories) and can create,
  rewrite, merge, or reorganize memories/plans on request. engram suspends during the
  session (the same handoff as `$EDITOR`) and reloads on exit. Requires the `claude` CLI
  on `PATH`; when it's missing the action shows a one-line hint instead of failing.
- Landing page for **engram.im** (`www/index.html`) — a single self-contained HTML
  page in the Classic Dark theme, with no build step or external assets. Intended to
  be served via Cloudflare Pages from `www/`; publishing is deferred (see SPEC §9).
- Project `.mcp.json` registering the `context7` and `sequential-thinking` MCP
  servers so Claude Code (not just VSCode) can use them; the context7 key is read
  from the `${CONTEXT7_API_KEY}` environment variable, so no secret is committed.

### Changed
- Transient footer messages are now color-coded by severity: warnings and deletions
  render as white text on red, and cancellations as dark brown text on emerald.
- The "index out of sync" warning now names its cause — how many files were added
  without a `MEMORY.md` index line, and/or how many index entries point to a
  deleted/renamed file — instead of just flagging that drift exists.
- Internal: split the ~1.9k-line `internal/tui/tui.go` into focused same-package
  files (`model`, `update`, `view`, `items`, `palette`, `render`, `style`, `editor`,
  `status`, `layout`, `navigation`, `reload`); no behavior change.

## [0.1.0] - 2026-06-22

First tagged release. v1 — local memory browsing — is feature-complete.

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

### Known gaps
- No public release or Homebrew tap published yet (release tooling is in place;
  publishing is deferred until v2).
- Team sharing over git (promote / pull, sync-status badges) is the next phase.

[Unreleased]: https://github.com/ertugrulhaskan/engram/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/ertugrulhaskan/engram/releases/tag/v0.1.0
