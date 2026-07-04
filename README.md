# engram

> A terminal UI for browsing, searching, and sharing your Claude Code memories.

`engram` is a fast, single-binary TUI that surfaces the memories Claude Code
keeps on your machine — across **all** your projects — in one searchable place.
Browse them, read them rendered as markdown, edit them in your `$EDITOR`, and
share the useful ones with your team over any git host.

The name comes from neuroscience: an *engram* is the physical trace a memory
leaves in the brain. That's exactly what these files are — the traces Claude
keeps so it can remember things across sessions.

> **Status:** Phase 1 (local browsing) + Phase 1.5 (assisted maintenance) complete —
> `v0.1.0`. Phase 2 (team sharing over git) core — `init-team`, `promote`, `pull`, and
> sync-status badges — is shipped on `main`; conflict-resolution UX is the remaining
> piece. The repo stays private until Phase 2 lands. See
> [ROADMAP.md](ROADMAP.md). Design details live in [SPEC.md](SPEC.md).
>
> **Website:** the landing page lives in-repo at [`www/index.html`](www/index.html), styled
> with Tailwind CSS (stock theme only; assets split into `www/css/` and `www/js/`) — run
> `npm run build:css` and commit the generated `www/css/styles.css` after changing classes
> (see [CONTRIBUTING.md](CONTRIBUTING.md) "Landing page"). It will be served at
> [engram.im](https://engram.im) (Cloudflare Pages; publishing deferred — see
> [SPEC.md](SPEC.md) §9).

## Why

Claude Code already stores memories as markdown files under
`~/.claude/projects/<project>/memory/`. But they're scattered one-folder-per-project,
they're awkward to read raw in a pager, and there's no good way to share the
team-useful ones with colleagues. `engram` fixes all three.

It is **not** a replacement for committing a `CLAUDE.md` to a repo — it covers
the gap that can't: cross-project memories, personal-vs-team layering, and a
proper UI.

## Install

> **Heads up — not published yet.** The repo stays private until Phase 2 ships,
> so the Homebrew tap and prebuilt binaries below aren't available, and
> `go install` needs repo access. For now, **build from a clone** (last option
> below). The other commands are how it will install once published.

**Homebrew** (macOS, from the `v0.1.0` release):

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
| `ctrl+p`   | command palette — three guides: `/` sources (`/memory`, `/plans`, `/files`, `/settings`), `@` for `@Claude`, and **`>` team commands** |
| `?`        | help — a keybinding cheat-sheet overlay (any key closes) |
| `q` / `ctrl+c` | quit                                |

**Team commands live under `>`** in the command palette (`ctrl+p`, then type `>`):
`>promote`, `>pull`, `>resolve`, `>withdraw`, and `>init <git-url>`. They act on the
selected memory and each surface a clear error if the team store isn't set up yet.

The left pane lists every memory found across all your projects, **grouped by
project** with a colored header per group; the right pane shows the selected
memory rendered as markdown. The command palette (`ctrl+p`) opens to three guide
rows — **`/`** for sources, **`@`** for the assistant, and **`>`** for team commands.
Typing `/` switches between sources — your memories, your plan-mode plans, and
**`/files`** (the read-only `CLAUDE.md` / `MEMORY.md` files Claude manages) — or opens
the config file; typing `@` launches **`@Claude`**; typing `>` runs a team command.

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

## Team sharing setup (Phase 2 — core shipped)

Team sharing lives under the **`>` command palette** (`ctrl+p`, then type `>`);
normal use stays a no-arg TUI. Set up the shared team store (a git repo your team
reads and writes) with **`>init <git-url>`** (or the equivalent `engram init-team
<git-url>` subcommand):

```sh
engram init-team <git-url>
```

This clones the team repo to `~/.config/engram/team/` and, if the repo is empty,
scaffolds `global/`, `projects/`, and `MEMORY.md`, then commits and pushes the
starter layout (a failed push is non-fatal — the local commit is kept, with a
retry hint).

Then, on the selected memory: **`>promote`** copies it into the store — a scope
dialog picks *this project* (keyed by its git remote) or *global*. engram stamps
the memory with an `engram:` frontmatter block (a durable id, scope, project,
owner — leaving Claude's own keys untouched) and commits + pushes the shared copy.
Before it pushes, engram **scans the memory for secrets** and, by default, blocks
the promote if it finds one — showing the redacted match with an option to
override. **`>pull`** brings the team's project memories down into their matching
local projects. When only the store moved and your copy is untouched, pull
**fast-forwards** it automatically; a copy you edited is left alone, and a genuine
divergence is flagged as a conflict rather than overwritten. Shared memories carry a
**sync-status badge** in the list — `✓` synced, `↓` incoming, `↑` ahead, `↕` conflict,
`!` missing — plus a muted `global`/`project` **scope chip**, so you can see each
one's state and bucket at a glance. **`>resolve`** opens both versions of a conflict
with git-style markers in your `$EDITOR` and writes your merge back, re-anchored so
"take theirs" reads as synced. **`>withdraw`** takes a shared memory back (after a
confirm) — if you're its owner: it removes the copy from the store, resets your memory
to personal, and, via a tombstone, removes it from teammates on their next pull
(`>promote` again puts it back).

The secret scan is tunable in `~/.config/engram/config.json`: `secretScanAction`
(`block` default · `block-strict` no override · `warn` · `off`) and
`secretScanScope` (`secrets` default · `secrets+pii`). It uses a curated rule set —
a guard, not a guarantee, so treat the override as a real decision.

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
- **Sync badge = team state.** Once you share, a team-scoped memory shows a filled
  pill for its state against the store. Thanks to a **sync anchor** (a content
  digest recorded when you last promoted/pulled), engram names a direction: `✓`
  synced, `↓` incoming (the store advanced, your copy is untouched — take it with `P`
  or `c`), `↑` ahead (you have unshared edits — `promote` to share), `↕` conflict
  (both sides moved — resolve with `c`), `!` missing (promoted but not in the store).
  A memory shared before this release has no anchor and shows a direction-less `●`
  differs. Personal memories show no pill, and the column vanishes until you set up a
  team store.
- **Scope chip = which bucket.** A muted `global` / `project` chip sits beside the
  sync pill so you can see whether a shared memory is team-wide (**global**) or tied
  to **this project** (keyed by its git remote) — the choice you make when you
  promote. Your user-wide rules in `~/.claude/CLAUDE.md` show up read-only under
  `/files`, alongside per-project `CLAUDE.md` and `MEMORY.md`.

## How it works

`engram` reads memory files from `~/.claude/projects/*/memory/*.md`. It supports
two on-disk shapes, because Claude Code uses both:

1. **YAML frontmatter** — `name`, `description`, `metadata.type`
2. **Plain markdown** — a `# Heading` title, with the one-line description pulled
   from the project's `MEMORY.md` index (`- [Title](file.md) — hook`)

Files are never modified except when you explicitly edit one. See
[SPEC.md](SPEC.md) for the full data model and the sharing design.

## Roadmap (short version)

- **Phase 1** — browse / search / view / edit local memories *(done — `v0.1.0`)*
- **Phase 1.5** — assisted maintenance: `@Claude`, read-only `/files` *(core in `v0.1.0`)*
- **Phase 2** — team sharing over git: `init-team`, promote / pull *(core shipped; sync-status badges + conflict resolve next)*
- **Phase 3** — other assistants' memories (Claude.ai, ChatGPT, …) as access allows

## Contributing

Contributions welcome! The codebase is small and deliberately layered:
`internal/memory` (discovery + parsing + file mutation, no UI) and `internal/tui`
(Bubble Tea UI, no file logic). See [CONTRIBUTING.md](CONTRIBUTING.md) for build
and test instructions, and [SPEC.md](SPEC.md) for the design. By participating
you agree to the [Code of Conduct](CODE_OF_CONDUCT.md).

## License

[MIT](LICENSE)
