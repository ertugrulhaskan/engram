# engram — Roadmap

Three phases, each independently shippable. Earlier phases stay useful even if
later ones never land.

---

## v1 — Local browsing *(in progress)*

Goal: a genuinely useful read/edit TUI with zero setup and no sharing.

- [x] Discover memories across all `~/.claude/projects/*/memory/` folders
- [x] Parse both on-disk shapes (YAML frontmatter **and** plain markdown + `MEMORY.md` index)
- [x] Two-pane TUI: searchable list + rendered markdown preview (Glamour)
- [x] Filter / search (`/`)
- [x] Edit selected memory in `$EDITOR` (`e`) and reload
- [x] `new` (`n`) and `delete` (`d`) actions
- [x] Filter by type (`t`)
- [x] Group/visually separate by project
- [ ] Sync `MEMORY.md` index on `new`/`delete` (so Claude picks up the changes)
- [ ] First tagged release: cross-platform binaries + Homebrew tap

## v2 — Team sharing over git

Goal: share the team-useful memories across people and projects, no servers.

- [ ] `engram init-team <git-url>` — set up the managed clone of the team repo
- [ ] Project identity via git remote URL (alias fallback)
- [ ] `promote` (single and multi-select) → commit + push
- [ ] `pull` → place team memories where Claude reads them + refresh `MEMORY.md`
- [ ] Personal vs team scope, enforced (personal never auto-syncs)
- [ ] Sync-status badges: `[+] new`, `[team ✓]`, `[team ●]`, `[team ↓]`, `[team ⚠]`
- [ ] Conflict resolution UX for `[team ⚠]`
- [ ] Global vs project-scoped team memories

## v3 — Other assistants

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
