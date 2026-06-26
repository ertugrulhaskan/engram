# engram — Roadmap

Three phases, each independently shippable. Earlier phases stay useful even if
later ones never land.

> **Phases are milestones, not release versions.** "Phase 1 / 1.5 / 2 / 3" name
> chunks of work; shipped releases follow [SemVer](https://semver.org), starting
> at `v0.1.0`. (Phase 1 and Phase 1.5 are released together as `v0.1.0`.)

---

## Phase 1 — Local browsing *(complete — shipped in `v0.1.0`)*

Goal: a genuinely useful read/edit TUI with zero setup and no sharing.

- [x] Discover memories across all `~/.claude/projects/*/memory/` folders
- [x] Parse both on-disk shapes (YAML frontmatter **and** plain markdown + `MEMORY.md` index)
- [x] Two-pane TUI: searchable list + rendered markdown preview (Glamour)
- [x] Filter / search (`/`)
- [x] Edit selected memory in `$EDITOR` (`e`) and reload
- [x] `new` (`n`) and `delete` (`d`) actions
- [x] Filter by type (`t`)
- [x] Group/visually separate by project (toggle by-project ⇄ by-type with `g`)
- [x] Sync `MEMORY.md` index on `new`/`delete`/`edit`; `R` reconciles a drifted index
- [x] Browse plan-mode plans too: multi-source switcher + command palette (`ctrl+p`),
      themed multi-pane UI with live theme switching, config persisted under XDG
- [x] Release tooling: GoReleaser (cross-platform binaries + Homebrew cask) + CI +
      tag-triggered release workflow — *publishing deferred until Phase 2*

## Phase 1.5 — Assisted memory maintenance *(core shipped in `v0.1.0`; one item left)*

Goal: hand the fiddly memory/plan upkeep to an AI, instead of fixing it by hand.
Independently shippable and strictly pre-Phase 2 (no sharing, no servers).

- [x] `@Claude` from the command palette — launch an interactive Claude Code session
      seeded with the selected project's memory/plan health, to repair drift the `R`
      reconcile can't (malformed frontmatter, broken `[[links]]`, memories stranded by a
      renamed project folder) and to create/rewrite/merge memories and plans on request
- [x] `/files` source — browse the global + per-project `CLAUDE.md` and each project's
      `MEMORY.md` read-only; edits are reserved for `@Claude`
- [ ] Other assistants behind the same `@<provider>` seam (overlaps Phase 3)

## Phase 2 — Team sharing over git

Goal: share the team-useful memories across people and projects, no servers.

- [x] `engram init-team <git-url>` — set up the managed clone of the team repo
- [ ] Project identity via git remote URL (alias fallback) — *`NormalizeRemote` built; no consumer/alias yet*
- [x] `promote` → commit + push *(single-select; multi-select pending)*
- [x] `pull` → place project team memories into matching local projects + refresh `MEMORY.md`
- [x] Personal vs team scope, enforced (personal never auto-syncs; pull never overwrites a personal file)
- [ ] Sync-status badges: `[+] new`, `[team ✓]`, `[team ●]`, `[team ↓]`, `[team ⚠]`
- [ ] Conflict resolution UX for `[team ⚠]`
- [x] Global vs project-scoped team memories *(promote writes `global/` or `projects/<key>/`)*

## Phase 3 — Other assistants

Goal: one place for memories beyond Claude Code — as each product allows.

- [ ] Pluggable "source" abstraction (Claude Code is the first source)
- [ ] Claude.ai memory (export/import until/unless an API exists)
- [ ] ChatGPT / Gemini memory (export/import)
- [ ] Read-only at first; editing/sharing per source as feasible

---

## Guiding constraints (all phases)

- Never modify a memory file unless the user explicitly edits/promotes it.
- Stay compatible with Claude Code's own reading of the files.
- No servers; sharing is plain git, host-agnostic.
- Single binary, small layered codebase.
