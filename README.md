# engram

> A terminal UI for browsing, searching, and sharing your Claude Code memories.

`engram` is a fast, single-binary TUI that surfaces the memories Claude Code
keeps on your machine — across **all** your projects — in one searchable place.
Browse them, read them rendered as markdown, edit them in your `$EDITOR`, and
(soon) share the useful ones with your team over any git host.

The name comes from neuroscience: an *engram* is the physical trace a memory
leaves in the brain. That's exactly what these files are — the traces Claude
keeps so it can remember things across sessions.

> **Status:** v1 (local browsing) complete — v0.1.0. See [ROADMAP.md](ROADMAP.md).
> Design details live in [SPEC.md](SPEC.md).
>
> **Website:** the landing page lives in-repo at [`www/index.html`](www/index.html) and
> will be served at [engram.im](https://engram.im) (Cloudflare Pages; publishing deferred —
> see [SPEC.md](SPEC.md) §9).

## Why

Claude Code already stores memories as markdown files under
`~/.claude/projects/<project>/memory/`. But they're scattered one-folder-per-project,
they're awkward to read raw in a pager, and there's no good way to share the
team-useful ones with colleagues. `engram` fixes all three.

It is **not** a replacement for committing a `CLAUDE.md` to a repo — it covers
the gap that can't: cross-project memories, personal-vs-team layering, and a
proper UI.

## Install

**Homebrew** (macOS, from the v0.1.0 release):

```sh
brew install ertugrulhaskan/tap/engram
```

**Go** (requires [Go](https://go.dev/dl/) 1.23+; works on Linux/Windows too):

```sh
go install github.com/ertugrulhaskan/engram@latest
```

**Prebuilt binaries** for macOS / Linux / Windows (amd64 + arm64) are attached to
each [release](https://github.com/ertugrulhaskan/engram/releases).

Or build from a clone:

```sh
git clone https://github.com/ertugrulhaskan/engram.git && cd engram
go mod tidy        # fetches dependencies (needs network, first time only)
go run .           # run it
go build -o engram # or build a binary
```

## Usage

Just run it:

```sh
engram
```

| Key        | Action                                  |
|------------|-----------------------------------------|
| `↑`/`↓` `j`/`k` | move through the list              |
| `pgup`/`pgdn` | page through the list                |
| `/`        | filter / search the list                |
| `tab`      | switch focus between list and preview   |
| `e`        | edit the selected memory in `$EDITOR`   |
| `n`        | create a new memory (in the current project) and open it |
| `d`        | delete the selected item (asks `y`/`n` first) |
| `t`        | cycle the type filter (all → user → feedback → project → reference → unknown) |
| `g`        | toggle grouping: by project ⇄ by type   |
| `R`        | reconcile the project's `MEMORY.md` index with its files (shown when out of sync) |
| `1`–`5`    | switch theme                            |
| `ctrl+p`   | command palette — browse `/memory`, `/plans`, `/files`, `/settings`, or type `@` for `@Claude` |
| `q` / `ctrl+c` | quit                                |

The left pane lists every memory found across all your projects, **grouped by
project** with a colored header per group; the right pane shows the selected
memory rendered as markdown. The command palette (`ctrl+p`) switches between
sources — your memories, your plan-mode plans, and **`/files`** (the read-only
`CLAUDE.md` / `MEMORY.md` files Claude manages) — opens the config file, and
(type `@`) launches **`@Claude`**.

> **`/files`** lists the global `~/.claude/CLAUDE.md`, each project's `CLAUDE.md`
> (when its directory resolves on disk), and each project's `MEMORY.md` index. These
> are **view-only** — `e`/`d` point you at `@Claude` instead of editing them directly,
> so the index and your instructions don't get hand-corrupted.

> `new`, `delete`, and `edit` keep the project's `MEMORY.md` index in sync, so
> Claude Code picks up the changes. When an index drifts, the warning names the
> cause — files added without an index line, and/or entries left by a deleted or
> renamed file — and `R` reconciles it.

> **`@Claude`** (palette → type `@`) hands off to an interactive
> [Claude Code](https://claude.com/claude-code) session, seeded with the selected
> project's memory/plan health, to fix what `R` can't (malformed frontmatter, broken
> `[[links]]`, memories stranded by a renamed project folder) and to create, rewrite,
> or merge memories on request. engram reloads when the session exits. Requires the
> `claude` CLI on `PATH`; without it the palette action shows a one-line hint.

## Understanding the list

- **Grouping.** Rows are grouped under a colored `▌ Group (N)` header with a
  count. For memories, press `g` to toggle between grouping **by project** (which
  Claude project a memory belongs to) and **by type**. Plans group **by recency**
  (Today / This week / Older). The selected row is marked with a `❯` cursor.
- **Color-coded badge = kind.** Each memory shows a colored badge for its type,
  taken from Claude's `metadata.type`:
  - `[user]` (blue) — a fact about you (role, preferences)
  - `[feedback]` (orange) — guidance on how to work
  - `[project]` (green) — something specific to that codebase
  - `[reference]` (purple) — a pointer to an external resource
  - `[other]` (gray) — no type recorded
- **No "global / all-projects" scope yet.** A cross-project scope (a memory that
  applies everywhere, or is shared with a team) is planned for **v2** — see
  [ROADMAP.md](ROADMAP.md). Your user-wide rules in `~/.claude/CLAUDE.md` show up
  read-only under `/files`, alongside per-project `CLAUDE.md` and `MEMORY.md`.

## How it works

`engram` reads memory files from `~/.claude/projects/*/memory/*.md`. It supports
two on-disk shapes, because Claude Code uses both:

1. **YAML frontmatter** — `name`, `description`, `metadata.type`
2. **Plain markdown** — a `# Heading` title, with the one-line description pulled
   from the project's `MEMORY.md` index (`- [Title](file.md) — hook`)

Files are never modified except when you explicitly edit one. See
[SPEC.md](SPEC.md) for the full data model and the sharing design.

## Roadmap (short version)

- **v1** — browse / search / view / edit local memories *(in progress)*
- **v2** — team sharing over git: promote / pull, sync-status badges
- **v3** — other assistants' memories (Claude.ai, ChatGPT, …) as access allows

## Contributing

Contributions welcome! The codebase is small and deliberately layered:
`internal/memory` (discovery + parsing + file mutation, no UI) and `internal/tui`
(Bubble Tea UI, no file logic). See [CONTRIBUTING.md](CONTRIBUTING.md) for build
and test instructions, and [SPEC.md](SPEC.md) for the design. By participating
you agree to the [Code of Conduct](CODE_OF_CONDUCT.md).

## License

[MIT](LICENSE)
