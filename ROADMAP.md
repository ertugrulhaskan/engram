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
      tag-triggered release workflow — *runs at the Phase 3 release*. **The cask step
      is currently broken:** it published the cask automatically for `v0.2.0` and
      `v0.2.1`, then began returning 403 at `v0.3.0` after the tap token was replaced,
      so `v0.3.0`'s cask was applied by hand. Binaries and the GitHub release itself
      are unaffected
- [x] Design-system pass over the TUI *(shipped in `v0.3.0`)* — three themes
      (Midnight / Paperback / CRT) over one named token set, every cell painted so
      a light theme survives a dark terminal, a two-row header block, a contextual
      status bar, one shared dialog anatomy, and confirms before every team write

## Phase 1.5 — Assisted memory maintenance *(complete — core in `v0.1.0`; the multi-assistant seam shipped in `v0.5.0`)*

Goal: hand the fiddly memory/plan upkeep to an AI, instead of fixing it by hand.
Independently shippable and strictly pre-Phase 2 (no sharing, no servers).

- [x] `@Claude` from the command palette — launch an interactive Claude Code session
      seeded with the selected project's memory/plan health, to repair drift the `R`
      reconcile can't (malformed frontmatter, broken `[[links]]`, memories stranded by a
      renamed project folder) and to create/rewrite/merge memories and plans on request
- [x] `/files` source — browse the global + per-project `CLAUDE.md` and each project's
      `MEMORY.md` read-only; edits are reserved for `@Claude`
- [x] Other assistants behind the same `@<provider>` seam — `@Claude`, `@Gemini`,
      `@Codex` and `@Copilot` share one registry, and the palette lists whichever are
      installed. Every entry must launch *interactively* (each of these CLIs also has a
      one-shot prompt flag that would quietly turn the handoff into a single answer) and
      must pass the memory dir through — `--add-dir`, or `--include-directories` for
      Gemini — since a session that can't reach `~/.claude` can't do the repair it was
      opened for

## Phase 2 — Team sharing over git *(shipped in v0.2.0 — direction badges, conflict resolution, secret-scan, `>` palette)*

Goal: share the team-useful memories across people and projects, no servers.

- [x] `engram init-team <git-url>` — set up the managed clone of the team repo
- [x] Project identity via git remote URL — consumed by promote/pull. **Alias fallback
      for remote-less repos shipped 2026-08-29:** `>alias <name>` keys such a project
      under `projects/alias/<name>/`, consulted only while git reports there is no
      remote (not on any git failure), so a project that later gains one promotes under
      its remote key from then on — what was promoted under the alias stays in that
      bucket until promoted again. Coordinating the *same* alias across teammates, and
      migrating an alias bucket once a remote appears, stay open (SPEC §10)
- [x] `promote` → commit + push — single memory, or **mark several with `space`** and
      promote them as **one commit and one push**, each memory landing in its own
      project. Every marked memory is secret-scanned, and a flagged one is decided
      individually. **`a` marks every memory the filters left**, so cycling `t` to a
      type and pressing `a` promotes that entire type
- [x] `withdraw` → take a promoted memory back: remove its copy from the team store, reset the local scope to personal, commit + push *(the reverse of `promote`; teammates who already pulled keep their copy)*
- [x] `pull` → place project team memories into matching local projects + refresh `MEMORY.md`; **fast-forward** a clean incoming update, leave a local-ahead copy, flag a real divergence
- [x] Personal vs team scope, enforced (personal never auto-syncs; pull never overwrites a personal file)
- [x] **Sync anchor** (`syncedHash` in the `engram:` block) enabling direction-aware states: `[synced]` / `[behind]` / `[ahead]` / `[conflict]` / `[missing]` *(`[unknown]` for pre-anchor memories)*
- [x] Conflict resolution UX (`>resolve`) — git-style markers in `$EDITOR`, re-anchored on save,
      with an **inline diff of the two sides in the confirm** before the editor opens
      *(the diff shipped 2026-08-29; SPEC §8.4)*
- [x] Scope chip (`global` / `project`) on shared rows
- [x] Global vs project-scoped team memories *(promote writes `global/` or `projects/<key>/`)*
- [x] Secret-scan guard on `promote` — block credentials from reaching the shared store (configurable, redacted findings)
- [x] Secret-scan sees a whole value and an unnamed one — the scan runs over
      *logical* lines, so a credential split by a soft wrap or a YAML block scalar is
      caught, and an entropy layer reports a raw generated value sitting under a name
      that gives nothing away. Thresholds measured against a real corpus (zero false
      positives on 105 files), not chosen; hex-shaped secrets stay a known gap
- [x] Team actions under the `>` command palette (`ctrl+p` → `>`); friendly error when `git` is missing
- [x] Auto-pull for global-scoped memories *(shipped in `v0.4.0`)* —
      `>pull` reconciles an existing local copy of a global memory with the same
      direction-aware safety as a project memory (fast-forward / ahead / conflict).
      A global memory you hold nowhere is still never *placed*: it belongs to no
      project, so the store stays its home until you promote it into one

## Phase 3 — Release / go public *(shipped — repo public, `v0.2.0` through `v0.5.0` released, [engram.im](https://engram.im) live)*

Goal: ship engram publicly — flip the repo public and make it installable.

- [x] Make the GitHub repo public
- [x] Push the release tag to trigger the GoReleaser publish workflow (`v0.2.0`,
      `v0.2.1`, then `v0.3.0`)
- [x] Publish the Homebrew tap (`ertugrulhaskan/tap/engram`) — *the published cask is
      current; the push was automatic through `v0.2.1`, but `v0.3.0` was bumped by
      hand (see the cask caveat under Phase 1)*
- [x] Deploy the landing page to [engram.im](https://engram.im) (Cloudflare Pages — live with SSL)
- [x] Verify install paths end-to-end (`brew install ertugrulhaskan/tap/engram`, `go install …@latest`, release binaries)

## Phase 4 — Other assistants *(tier 1 — local instruction files — shipped in `v0.4.0`; tier 2 open)*

Goal: one place for AI context beyond Claude Code — local files first, server-side
memories as each product allows.

- [x] Pluggable "source" abstraction (Claude Code is the first source) — *satisfied
      2026-09-02, by the shape that grew rather than the Go `interface` this item
      imagined*. Two halves: **capability**, as `source.Caps` declared by each data
      package and consumed through one TUI gate (`20e731a`), whose zero value grants
      nothing; and **discovery**, as data-driven tables plus a `Provider` dimension.
      The testable bar was "adding a new local-file source needs no `internal/tui`
      change", and it was measured rather than argued: a fake `windsurf` provider cost
      1 constant + 1 table row + 1 canary line, with zero TUI edits.

      **What stays unwritten, deliberately:** the load-path interface. The tables
      generalise "a file, or a directory of files, at a known path under a project or
      the home dir" and nothing else, and every vendor added so far is a provider
      *within* the files source, not a new collection alongside memories/plans/files.
      Writing the interface now would still be designing against one real and several
      hypothetical implementations. Tier 2 below is the first thing that wouldn't fit,
      and it reopens the question if and when a vendor ships a memory API
- [x] **`AGENTS.md`** (Codex / cross-tool standard) — read-only in `/files`, with an
      `agents` badge. Found for projects engram already knows about (those with a
      `~/.claude/projects/` folder); a repo never opened in Claude Code doesn't appear
- [x] **`GEMINI.md`** (Gemini CLI) and **`.github/copilot-instructions.md`** (GitHub
      Copilot) — read-only in `/files` with `gemini` / `copilot` badges, on the same
      `projectRuleFiles` table, so both inherit the discovery limit above. **This item
      covers each project's own file and nothing else** — the vendors' global equivalents
      sit outside the `~/.claude` tree the walk is anchored to and are their own item
      below, now also done
- [x] Cursor rules (`.cursor/rules/*.mdc`) — and Copilot's path-scoped
      `.github/instructions/*.instructions.md` *(shipped in `v0.4.0`)*.
      Both are directories of frontmatter-bearing files, covered by a second table
      (`projectRuleDirs`) beside `projectRuleFiles`: recursive within the rules dir
      (Cursor documents organizing rules in folders), suffix-strict (`.mdc` /
      `.instructions.md` — Cursor itself ignores a plain `.md` there), frontmatter split
      off the preview with the scoping surfaced as an "applies to …" line. A non-empty
      rules dir also qualifies a scan-root project. Deliberately out: monorepo-nested
      `.cursor/rules` in subdirectories (a repo-tree walk per 2s poll tick), the legacy
      `.cursorrules` (absent from current Cursor docs), and Cursor/Copilot user-level
      rules (app-internal storage, no file)
- [x] Lift the Claude-anchored discovery limit — a configured `scanRoots` list, so
      projects Claude Code has never opened can appear. Each root and its immediate
      children are checked; a directory qualifies only if it carries an instruction file.
      Depth is 1 on purpose: the reload poll re-runs the scan every 2s
- [x] The vendors' **global** instruction files — `~/.gemini/GEMINI.md` and
      `~/.codex/AGENTS.md`, read from a `globalRuleFiles` table and listed in the `global`
      scope beside `~/.claude/CLAUDE.md`. No configured-file notion was needed after all:
      both sit at fixed paths under the home dir, so the same table pattern that covers the
      per-project files covers these. They are read straight from the home dir, so they show
      up with no `~/.claude` and no scan roots. Copilot is absent because it has no home-dir
      instructions file (VS Code stores user-level instructions in profile settings), and
      Codex's `CODEX_HOME` / `AGENTS.override.md` precedence isn't honoured
- [ ] Server-side memories — Claude.ai / ChatGPT / Gemini app: export/import,
      since none of them exposes a memory API today (until/unless that changes)
- [x] Read-only at first; editing/sharing per source as feasible *(decided and enforced
      2026-08-29)* — memories: everything; plans: view + delete; files: read-only
      (repairs via the `@` assistant handoff). Capability is a `source.Caps` each data package declares
      (`memory.Caps`, `plan.Caps`, `memory.DocsCaps`), wired once in the TUI and asked
      through one gate; the zero value grants nothing, so a new source is read-only until
      granted, and the controls row is derived from the same struct so it can only
      advertise a key that runs. The test for granting edit to any source: engram can
      keep the promise "stay compatible with the tool that owns this file". Tier-2 imports
      become regular memories once imported (SPEC §8.3)

---

## Guiding constraints (all phases)

- Never modify a memory file unless the user explicitly edits/promotes it.
- Stay compatible with Claude Code's own reading of the files.
- No servers; sharing is plain git, host-agnostic.
- Single binary, small layered codebase.
