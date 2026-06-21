# engram

> A terminal UI for browsing, searching, and sharing your Claude Code memories.

`engram` is a fast, single-binary TUI that surfaces the memories Claude Code
keeps on your machine — across **all** your projects — in one searchable place.
Browse them, read them rendered as markdown, edit them in your `$EDITOR`, and
(soon) share the useful ones with your team over any git host.

The name comes from neuroscience: an *engram* is the physical trace a memory
leaves in the brain. That's exactly what these files are — the traces Claude
keeps so it can remember things across sessions.

> **Status:** v1 in progress (local browsing). See [ROADMAP.md](ROADMAP.md).
> Design details live in [SPEC.md](SPEC.md).

## Why

Claude Code already stores memories as markdown files under
`~/.claude/projects/<project>/memory/`. But they're scattered one-folder-per-project,
they're awkward to read raw in a pager, and there's no good way to share the
team-useful ones with colleagues. `engram` fixes all three.

It is **not** a replacement for committing a `CLAUDE.md` to a repo — it covers
the gap that can't: cross-project memories, personal-vs-team layering, and a
proper UI.

## Install

> Requires [Go](https://go.dev/dl/) 1.23+ to build from source. Prebuilt
> binaries and a Homebrew tap are planned for the first tagged release.

```sh
go install github.com/ertughaskan/engram@latest
```

Or from a clone:

```sh
git clone <your-repo-url> engram && cd engram
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
| `↑`/`↓` `j`/`k` | move through the memory list        |
| `/`        | filter / search memories                |
| `enter`    | (in filter) apply the filter            |
| `tab`      | switch focus between list and preview   |
| `e`        | edit the selected memory in `$EDITOR`   |
| `n`        | create a new memory (in the current project) and open it |
| `d`        | delete the selected memory (asks `y`/`n` first) |
| `t`        | cycle the type filter (all → user → feedback → project → reference → unknown) |
| `q` / `ctrl+c` | quit                                |

The left pane lists every memory found across all your projects, **grouped by
project** with a colored header per group; the right pane shows the selected
memory rendered as markdown.

> Note: `new` and `delete` change the memory files but do **not** yet update the
> project's `MEMORY.md` index — that sync is a tracked follow-up.

## Understanding the list

- **Grouping = project.** Memories are grouped under a `▌ project-name (N)` header
  showing which Claude project they belong to and how many it has. Everything in
  the list is **project-scoped** — it lives under one project's `memory/` folder.
- **Badge = kind.** Each memory is tagged with its type, taken from Claude's
  `metadata.type`:
  - `[user]` — a fact about you (role, preferences)
  - `[feedback]` — guidance on how to work
  - `[project]` — something specific to that codebase
  - `[reference]` — a pointer to an external resource
  - `[other]` — no type recorded
- **No "global / all-projects" scope yet.** A cross-project scope (a memory that
  applies everywhere, or is shared with a team) is planned for **v2** — see
  [ROADMAP.md](ROADMAP.md). Your user-wide rules in `~/.claude/CLAUDE.md` are a
  separate file that engram does not read in v1.

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
