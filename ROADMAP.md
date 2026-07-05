# engram — Roadmap

Several phases, each independently shippable. Earlier phases stay useful even if
later ones never land.

> **Phases are milestones, not release versions.** "Phase 1 / 1.5 / 2 / 3 / 4" name
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
      tag-triggered release workflow — *runs at the Phase 3 release*

## Phase 1.5 — Assisted memory maintenance *(core shipped in `v0.1.0`; one item left)*

Goal: hand the fiddly memory/plan upkeep to an AI, instead of fixing it by hand.
Independently shippable and strictly pre-Phase 2 (no sharing, no servers).

- [x] `@Claude` from the command palette — launch an interactive Claude Code session
      seeded with the selected project's memory/plan health, to repair drift the `R`
      reconcile can't (malformed frontmatter, broken `[[links]]`, memories stranded by a
      renamed project folder) and to create/rewrite/merge memories and plans on request
- [x] `/files` source — browse the global + per-project `CLAUDE.md` and each project's
      `MEMORY.md` read-only; edits are reserved for `@Claude`
- [ ] Other assistants behind the same `@<provider>` seam (overlaps Phase 4)

## Phase 2 — Team sharing over git *(shipped in v0.2.0 — direction badges, conflict resolution, secret-scan, `>` palette)*

Goal: share the team-useful memories across people and projects, no servers.

- [x] `engram init-team <git-url>` — set up the managed clone of the team repo
- [x] Project identity via git remote URL — *consumed by promote/pull; alias fallback for remote-less repos pending*
- [x] `promote` → commit + push *(single-select; multi-select pending)*
- [x] `withdraw` → take a promoted memory back: remove its copy from the team store, reset the local scope to personal, commit + push *(the reverse of `promote`; teammates who already pulled keep their copy)*
- [x] `pull` → place project team memories into matching local projects + refresh `MEMORY.md`; **fast-forward** a clean incoming update, leave a local-ahead copy, flag a real divergence
- [x] Personal vs team scope, enforced (personal never auto-syncs; pull never overwrites a personal file)
- [x] **Sync anchor** (`syncedHash` in the `engram:` block) enabling direction-aware badges: `✓` synced / `↓` incoming / `↑` ahead / `↕` conflict / `!` missing *(direction-less `●` differs for pre-anchor memories)*
- [x] Conflict resolution UX (`>resolve`) — git-style markers in `$EDITOR`, re-anchored on save
- [x] Scope chip (`global` / `project`) on shared rows
- [x] Global vs project-scoped team memories *(promote writes `global/` or `projects/<key>/`)*
- [x] Secret-scan guard on `promote` — block credentials from reaching the shared store (configurable, redacted findings)
- [x] Team actions under the `>` command palette (`ctrl+p` → `>`); friendly error when `git` is missing
- [ ] Auto-pull for global-scoped memories *(today `>pull` walks `projects/` only; global updates are taken via `>resolve`)*

## Phase 3 — Release / go public *(shipped — repo public, `v0.2.0` + `v0.2.1` released)*

Goal: ship engram publicly — flip the repo public and make it installable.

- [x] Make the GitHub repo public
- [x] Push the release tag to trigger the GoReleaser publish workflow (`v0.2.0`, then `v0.2.1`)
- [x] Publish the Homebrew tap (`ertugrulhaskan/tap/engram`)
- [ ] Deploy the landing page to [engram.im](https://engram.im) (Netlify) — *DNS in progress*
- [x] Verify install paths end-to-end (`brew install ertugrulhaskan/tap/engram`, `go install …@latest`, release binaries)

## Phase 4 — Other assistants

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
