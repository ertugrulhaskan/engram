# engram — working rules

engram is a single-binary Go TUI for browsing (and, in Phase 2, sharing) Claude Code
memories. Design source of truth: [SPEC.md](SPEC.md). Phases: [ROADMAP.md](ROADMAP.md).
Build/test rules: [CONTRIBUTING.md](CONTRIBUTING.md). Read SPEC.md before changing
behavior.

## Before you commit — review gate

Before every commit, run these in order and **share findings + highlight the important
points after each step** (don't fold them into one silent summary):

1. **Review the code** with `context7` (verify Bubble Tea / lipgloss / glamour and any
   other library APIs against current docs — don't trust memory) and `sequential-thinking`
   (reason through correctness, edge cases, and the `internal/*` layering rules below).
   **The repo ships no MCP config** — there's no `.mcp.json`, because a project-scoped one
   can only pass the context7 key as `${CONTEXT7_API_KEY}`, which isn't expanded at
   MCP-launch time and reaches the server as a literal string. Register both servers
   yourself instead, with your own key:
   `claude mcp add context7 -s user -e CONTEXT7_API_KEY=… -- npx -y @upstash/context7-mcp@latest`
   and `claude mcp add sequential-thinking -s user -- npx -y @modelcontextprotocol/server-sequential-thinking`.
2. **Review the working diff with `/code-review`.** This *is* agent-runnable —
   verified 2026-08-23, when it ran on `002786d` and returned five findings, three
   of which a careful manual pass had already missed. (This file previously claimed
   an agent couldn't run it; that was wrong.) Run the real skill. If it ever refuses,
   fall back to a manual diff pass and **say so explicitly**, never wording it as
   though the skill ran. `/code-review ultra` is the exception — that one is
   user-triggered and billed, so ask the maintainer rather than attempting it.
3. **`/security-review`** over the pending branch changes. This one *is* agent-runnable —
   run the real skill, never a folded-in inline assessment.
4. **Sync the docs** — see "Keep the docs in sync" below (CHANGELOG, ROADMAP, SPEC,
   README, this file, memories, plans).

This is in addition to the formatting/vet/test gate in "Code rules". **That half is the
only machine-enforced part:** `.github/workflows/ci.yml` runs `gofmt -l .`, `go vet ./...`,
and `go test ./...` on every push to `main` and every PR. Steps 1–4 have no enforcement at
all — `.git/hooks/` holds only the stock samples — so the discipline lives here.

## Keep the docs in sync — before you commit

When a change alters behavior, structure, or status, update the affected docs in
the *same* change. Before committing, ask: "did this make any of these wrong?"

- **CHANGELOG.md** — record every user-facing change (Keep a Changelog format).
- **ROADMAP.md** — tick/move an item when its capability lands or its status changes.
- **SPEC.md** — update the data model (§6), module layout (§8), or design sections
  when types, packages, or behavior change.
- **README.md** — keep the keybinding table, install steps, and feature list matching
  the actual TUI.
- **BRAND.md** — the voice and claims rules public copy is checked against: the site and
  `llms.txt` today, README and TUI strings as they are touched. A naming, platform, or proof
  rule changes there first; the copy follows.
- **The site's derived files** (`www/robots.txt`, `www/sitemap.xml`, `www/llms.txt`, and
  the two `application/ld+json` blocks in `www/index.html`) — these restate what's on the
  landing page, so a pitch/install/platform change must update them too. The `FAQPage`
  answers must stay word-for-word identical to the visible FAQ cards —
  `.github/scripts/verify-site.py` now enforces that one, so it fails in CI rather than
  reaching a crawler.
- **`www/_headers`** — its CSP pins a SHA-256 hash of each inline `<script>`, so editing
  one of those scripts without recomputing the hash breaks the page in production only.
  Adding a page also means an `@source` line in `www/css/input.css` and a CSS rebuild.
  The recompute command is in `_headers` itself.
- **Memories** (the project's `~/.claude/.../memory/` files) — when a project decision
  changes, update the relevant memory and its `MEMORY.md` line.

This list exists because docs drift silently otherwise — that's how ROADMAP fell
behind the shipped index-sync and release work.

## Code rules (detail in CONTRIBUTING.md)

- **Layering:** every package outside `internal/tui` contains no UI — `memory`, `plan`,
  `config`, `team`, `secrets`, `source`; `internal/tui` contains no file/IO logic. Don't
  cross the line.
- **Never modify a user's memory files** except on an explicit user action
  (edit/create/delete/promote/withdraw). Only ever *add* frontmatter keys engram owns;
  never rewrite Claude's fields.
- Run `gofmt -w .` before committing, and keep `go vet ./...` / `go test ./...` green.
  CI (`.github/workflows/ci.yml`) enforces all three, but only *after* the push — running
  them locally is what saves a red build.
- Commit messages: conventional prefixes, present tense ("add x", not "added x").

## Release / publishing

Phase 3 (going public) is **done**: the repo is public, `v0.2.0` through `v0.5.0` are released
(binaries + Homebrew tap live), and the site is **live at engram.im on Cloudflare Pages** (SSL).
**For any future release, don't run the publish steps unprompted** — pushing a `v*` tag
fires the GoReleaser workflow and cuts a public release (push the *specific* tag, never
`git push --tags`), so do it only when the maintainer explicitly asks. Mechanics: SPEC §9
and the "Releasing" section of
CONTRIBUTING.md.
