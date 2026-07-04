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
2. **`/code-review`** on the working diff; address what it finds.
3. **`/security-review`** over the pending branch changes.
4. **Sync the docs** — see "Keep the docs in sync" below (CHANGELOG, ROADMAP, SPEC,
   README, this file, memories, plans).

This is in addition to the `gofmt -w . && go vet ./... && go test ./...` gate in "Code
rules". It's followed guidance, not a hook — the review skills are interactive and can't
run unattended, so the discipline lives here.

## Keep the docs in sync — before you commit

When a change alters behavior, structure, or status, update the affected docs in
the *same* change. Before committing, ask: "did this make any of these wrong?"

- **CHANGELOG.md** — record every user-facing change (Keep a Changelog format).
- **ROADMAP.md** — tick/move an item when its capability lands or its status changes.
- **SPEC.md** — update the data model (§6), module layout (§8), or design sections
  when types, packages, or behavior change.
- **README.md** — keep the keybinding table, install steps, and feature list matching
  the actual TUI.
- **Memories** (the project's `~/.claude/.../memory/` files) — when a project decision
  changes, update the relevant memory and its `MEMORY.md` line.

This list exists because docs drift silently otherwise — that's how ROADMAP fell
behind the shipped index-sync and release work.

## Code rules (detail in CONTRIBUTING.md)

- **Layering:** `internal/memory`, `internal/plan`, and `internal/config` contain no
  UI; `internal/tui` contains no file/IO logic. Don't cross the line.
- **Never modify a user's memory files** except on an explicit user action
  (edit/create/delete/promote/withdraw). Only ever *add* frontmatter keys engram owns;
  never rewrite Claude's fields.
- Run `gofmt -w .`, `go vet ./...`, and `go test ./...` before committing.
- Commit messages: conventional prefixes, present tense ("add x", not "added x").

## Release / publishing

Phase 3 — going public — is in progress: the repo is being made public and `v0.2.0`
released (binaries + Homebrew tap, deploy engram.im). The release tooling is built and
verified. **Don't run the publish steps unprompted** — no `git push --tags` (it fires the
GoReleaser workflow), no GitHub Release, no visibility change — unless the maintainer
explicitly asks in that turn. Mechanics: SPEC §9 and the "Releasing" section of
CONTRIBUTING.md.
