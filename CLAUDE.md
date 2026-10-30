# engram — working rules

engram is a single-binary Go TUI for browsing Claude Code memories and sharing them with
a team over plain git. Design source of truth: [SPEC.md](SPEC.md). Phases:
[ROADMAP.md](ROADMAP.md). Build/test rules: [CONTRIBUTING.md](CONTRIBUTING.md). Read
SPEC.md before changing behavior.

## Before you commit — review gate

Before every commit, run these in order and **share findings + highlight the important
points after each step** (don't fold them into one silent summary):

1. **Check library APIs and layering.** Verify Bubble Tea / lipgloss / glamour and any
   other library APIs against current docs with `context7` — don't trust memory — and
   check the change against the `internal/*` layering rules below.
   **The repo ships no MCP config** — there's no `.mcp.json`, because a project-scoped one
   can only pass the context7 key as `${CONTEXT7_API_KEY}`, and Claude Code expands that
   from its own environment at launch: an editor started without the variable set hands
   the server the literal placeholder, which it rejects as an invalid key. Register the
   server yourself instead, with your own key:
   `claude mcp add context7 -s user -e CONTEXT7_API_KEY=… -- npx -y @upstash/context7-mcp@latest`.
2. **Review the working diff with `/code-review high`.** The skill is agent-runnable:
   invoke the real skill yourself — it has caught defects a careful manual pass missed.
   Name the level every time, because a bare `/code-review` reuses whichever level was
   typed last. `/code-review max` is the exception — it is the costliest level and starts
   many reviewer subagents, so ask the maintainer rather than running it.
3. **`/security-review`** over the pending branch changes. Also agent-runnable: run the
   real skill, never a folded-in inline assessment.
4. **Sync the docs** — see "Keep the docs in sync" below (CHANGELOG, ROADMAP, SPEC,
   README, this file, memories, plans).

If either review skill refuses or is unavailable, fall back to a manual pass and **say so
explicitly**, never wording it as though the skill ran.

This is in addition to the formatting/vet/test gate in "Code rules". **That half is the
only machine-enforced part:** on every push to `main` and every PR,
`.github/workflows/ci.yml` runs `gofmt -l .`, `go vet ./...`, `go test ./...` and
`govulncheck -scan module` in its `build` job, builds and tests again on the Go version
`go.mod` declares in its `floor` job (pinned with `GOTOOLCHAIN=local`, or it would
silently upgrade past the floor it is checking), and in its `site` job rebuilds `www/css/styles.css` to fail on a stale commit
of it, then runs `.github/scripts/verify-site.py` for the CSP hashes, the Tailwind
`@source` lines and FAQ/JSON-LD parity. Steps 1–4 have no enforcement at all —
`.git/hooks/` holds only the stock samples — so the discipline lives here.

## Keep the docs in sync — before you commit

When a change alters behavior, structure, or status, update the affected docs in
the *same* change. Before committing, ask: "did this make any of these wrong?"

- **CHANGELOG.md** — record every user-facing change (Keep a Changelog format).
- **ROADMAP.md** — tick/move an item when its capability lands or its status changes.
- **SPEC.md** — update the data model (§6), module layout (§8), or design sections
  when types, packages, or behavior change.
- **README.md** — keep the keybinding table, install steps, and feature list matching
  the actual TUI. **`docs/tui.png` drifts silently** — nothing in CI looks at it, so a UI
  change or a version bump needs a recapture (`docs/demo/README.md`), not a text edit.
- **BRAND.md** — the voice and claims rules public copy is checked against: the site and
  `llms.txt` today, README and TUI strings as they are touched. A naming, platform, or proof
  rule changes there first; the copy follows.
- **The site's derived files** (`www/robots.txt`, `www/sitemap.xml`, `www/llms.txt`, and
  the two `application/ld+json` blocks in `www/index.html`) — these restate what's on the
  landing page, so a pitch/install/platform change must update them too. The `FAQPage`
  answers must stay word-for-word identical to the visible FAQ cards —
  `.github/scripts/verify-site.py` enforces that one, so it fails in CI rather than
  reaching a crawler.
- **`www/_headers`** — its CSP pins a SHA-256 hash of each inline `<script>`, so editing
  one of those scripts without recomputing the hash breaks the page in production only.
  Adding a page also means an `@source` line in `www/css/input.css` and a CSS rebuild.
  The recompute command is in `_headers` itself.
- **Memories** (the project's `~/.claude/.../memory/` files) — when a project decision
  changes, update the relevant memory and its `MEMORY.md` line.

This list exists because docs drift silently otherwise.

## Code rules (detail in CONTRIBUTING.md)

- **Layering:** every package outside `internal/tui` contains no UI — `memory`, `plan`,
  `config`, `team`, `secrets`, `source`; `internal/tui` contains no file/IO logic. Don't
  cross the line.
- **Never modify a user's memory files** except on an explicit user action
  (edit/create/delete/promote/withdraw). Only ever *add* frontmatter keys engram owns;
  never rewrite Claude's fields.
- Run `gofmt -w .` before committing, and keep `go vet ./...` / `go test ./...` green.
  CI (`.github/workflows/ci.yml`) enforces all three, but only *after* the push — running
  them locally is what saves a red build. The same applies to the site checks: run
  `npm run build:css` and `python3 .github/scripts/verify-site.py` after touching `www/`.
- Commit messages: conventional prefixes, present tense ("add x", not "added x").

## Release / publishing

Phase 3 (going public) is **done**: the repo is public, tagged releases ship binaries and the
Homebrew tap, and the site is **live at engram.im on Cloudflare Pages** (SSL).
**For any future release, don't run the publish steps unprompted** — pushing a `v*` tag
fires the GoReleaser workflow and cuts a public release (push the *specific* tag, never
`git push --tags`), so do it only when the maintainer explicitly asks. Mechanics: SPEC §9
and the "Releasing" section of
CONTRIBUTING.md.
