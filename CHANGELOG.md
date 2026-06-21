# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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

### Known gaps
- `new` / `delete` do not yet update the project's `MEMORY.md` index.
- No tagged release or Homebrew tap yet.

[Unreleased]: https://example.com/your-repo/compare/main...HEAD
