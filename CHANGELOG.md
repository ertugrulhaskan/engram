# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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
