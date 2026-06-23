# engram — Open Questions

Design questions for v2 (team sharing). Resolved items are reflected in
[SPEC.md](SPEC.md) §7; for sequencing see [ROADMAP.md](ROADMAP.md).

> Started 2026-06-22. Core v2 design locked 2026-06-22 — see **Resolved** below.

---

## Still open / deferred

Deliberately punted past the first v2 cut — none block starting v2:

- **Whole-type promote** (e.g. "promote all feedback at once"). Single +
  multi-select promote ships first. (SPEC §10 future.)
- **Inline diff for `[team ⚠]`.** v2 opens both versions in `$EDITOR`; an inline
  diff view comes later.
- **Monorepo sub-keys.** Subprojects sharing one remote share one bucket in v2;
  per-subdir keys are a later refinement.
- **No-remote alias coordination.** A no-remote project falls back to a
  user-assigned alias, but two teammates must pick the *same* alias for their
  memories to meet. v2 leaves this to out-of-band agreement; a shared alias map
  in the team repo is a possible later fix.
- **Repo URL onboarding.** Teammates get the team repo URL out-of-band
  (Slack/README) for v2; an `engram invite` helper is out of scope.

---

## Resolved — 2026-06-22 (folded into SPEC §7)

**Interface & setup**
- Team ops model = **hybrid**: `engram init-team <git-url>` is a one-time CLI
  subcommand; `promote`/`pull` are in-TUI keybind actions. init-team is the only
  subcommand; daily use stays a no-arg TUI.
- `init-team` **scaffolds** an empty repo (`global/`, `projects/`, `MEMORY.md`).
- Managed clone lives at **`~/.config/engram/team/`** (XDG, beside existing config).
- **No engram-level auth or servers**; access delegated to the git host. Push/pull
  use the user's existing git credentials; clear errors when missing.

**Identity & matching**
- A promoted memory gets a **stable `engram.id` UUID** in the engram frontmatter
  block. Identity tracks across slug renames/edits and across machines, so a
  rename *updates* the team copy instead of orphaning it. The slug is just the
  filename.
- **Collision on pull:** never overwrite a personal file; a differing same-id (or
  same-name) file → `[team ⚠]`; identical content = no-op.

**Project identity**
- Remote URLs **normalized** to canonical lowercased `host/path` (strip protocol,
  `user@`, trailing `.git`, trailing `/`), so `git@…`, `https://…`, and `ssh://…`
  all collapse to one slug, e.g. `github.com/acme/app`.

**Sync mechanics**
- **Manual sync:** explicit `pull`; engram never auto-pulls. Launch does a cheap
  check and badges `[team ↓]`; files are placed only on `pull`.
- **Rejected push:** auto `git pull --rebase` + retry once, then hand off on conflict.
- **Conflict UX:** open both versions in `$EDITOR` (inline diff deferred).
- **Promote scope:** modal at promote time, defaulting to the current project,
  with a "global" option.

**Carryover from v1**
- `new`/`delete`/`edit` already sync `MEMORY.md`, and `R` reconciles a drifted
  index — shipped in v1. (This was the prerequisite for `pull`'s index refresh.)
