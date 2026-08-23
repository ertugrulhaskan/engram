# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

> Releases are versioned with SemVer (`v0.1.0`, …). The "Phase 1 / 1.5 / 2"
> labels in [ROADMAP.md](ROADMAP.md) are development milestones, not versions.

## [Unreleased]

### Added
- **Promote a whole type at once.** `a` marks every memory in the list. Because
  `t` already narrows the list to one type, cycling to `feedback` and pressing `a`
  marks exactly the feedback memories — the type filter *is* the selection, so
  there is no second "promote a type" path to keep in step with the batch one.
  Everything downstream is the batch promote that already existed: one commit,
  one push, every memory secret-scanned.

  It scales `space` up rather than sitting beside it. Pressing `a` on a list whose
  memories are all marked unmarks exactly those, so the key is never dead; on a
  partly-marked list it completes the set instead, which is what keeps it from
  discarding marks made by hand.

  "The list" means everything the filters left, not the rows that fit on screen —
  fifty matches are fifty marks even where twenty are visible. The filters are the
  selection; where the fold falls is not.

  **Marks outside the filters are never touched.** That is what lets a batch span
  two types — mark one, switch the filter, mark another — and it is also the risk,
  so the status line names what it acted on (`marked 3 feedback memories`) and
  appends the running total whenever any mark sits outside that list
  (`· 4 marked in all`). A batch wider than what you are looking at can't be a
  surprise at the promote confirm.

  The type is named only when the type filter *alone* built the list. Add a search
  on top and the line keeps its "in the list" hedge — `marked 1 feedback memory`
  would otherwise claim a whole type when two of its three memories are not
  marked. The name it uses is the one on the row badges, so a `other`-badged
  memory never reports as `unknown`.

- **Promote several memories at once, as one commit.** `space` marks a memory (and
  moves down, so marking a run costs one keystroke each); `esc` clears the marks.
  With marks set, `promote` acts on the marked set instead of the cursor row, and
  the whole set is written, committed and pushed **once** — promoting five memories
  used to mean five commits and five pushes for what you did as a single act.

  The scope modal offers **their own projects** — each memory lands in its own
  project key, so a batch can span several — or `global` for all of them. It states
  the real spread before you choose (how many distinct keys, and how many have no
  git remote and will therefore go global). A mark hidden by a search filter still
  promotes: dropping it would quietly promote less than you marked.

  **Every memory in the batch is secret-scanned**, not just the first, and a flagged
  one is decided on its own terms: `y` includes it, `n` skips it (leaving that one
  personal), `esc` abandons the batch. Skipping works under `block-strict` too, since
  it overrides nothing — it promotes less. Whatever survives still lands as one
  commit, and the result line reports both what was skipped and what was included
  past a finding, rather than reading as a clean sweep.

  Nothing is written until the whole batch has been prepared, so a batch that can't
  go through — an unsafe project key, an unreadable memory, or two memories that
  would claim the same path in the store — leaves every local file untouched instead
  of half-applying. That last case is new to batching: on disk neither copy exists
  yet, so the existing collision check couldn't have seen it. Paths are compared
  case-folded, because `Notes.md` and `notes.md` are one file on macOS and Windows
  and an exact compare would have lost one of the two.

  The mark column appears only while something is marked, so every single-memory
  view keeps exactly the layout it had before.

- **The promote secret scan now catches a secret that doesn't look like one.** The
  scan read a value two ways — is it a known key format, and is it sitting under a
  secret-ish name — which left one gap wide open: a raw generated value under a name
  that gives nothing away (`deploy: Xk9mPq2…`). Neither question can reach it, and
  that is exactly the shape a hand-written memory tends to carry.

  A third layer now judges the value on its own: a run of 28 or more credential
  characters scoring at least 4.4 bits of Shannon entropy is reported as
  `high-entropy-string`, redacted like every other finding. It speaks only when the
  other two layers are silent, so a recognized key is never listed twice.

  The thresholds were measured against a real corpus rather than picked, and the
  measurement is what shaped the design. Entropy alone turned out to be unusable: a
  filesystem path is a single unbroken run — `/` and `-` are both base64 characters —
  and a long one scores 4.45–4.62, above any bar a 28-character key must also clear.
  Ten of ten findings on a 105-file corpus were paths. Raising the threshold would
  have traded those for missed keys, so the fix is structural instead: a run made of
  five or more separator-joined words is text, not a token, and is vetoed regardless
  of how it scores. That takes the same corpus to **zero** findings while costing, at
  worst, one real key in 12,000 — measured over 200,000 synthetic values per format,
  not assumed. `internal/secrets.TestEntropyCorpus` re-runs the whole measurement
  against any tree of memories via `ENGRAM_CORPUS_ROOT`.

  Deliberately still missed: hex-encoded secrets, which are indistinguishable from
  the SHAs and UUIDs that fill real memories, and anything under 28 characters. The
  scan stays a guard with an informed override, not a guarantee.

### Fixed
- **The promote secret scan now catches a credential split across lines.** The
  scanner read one physical line at a time, so a token broken by a soft wrap, a
  YAML block scalar, or a backslash continuation matched no rule — no rule ever saw
  the whole value. That was structural, not a missing pattern: no additional regex
  could have closed it.

  The same rules now also run over *logical* lines, with the breaks that merely wrap
  a value removed. A break counts as a wrap only when runs of credential characters
  meet across it and at least one carries a digit or runs long — enough to reassemble
  `AKIA1234` + `5678ABCDEFGH`, not enough to fuse two ordinary sentences. A match
  spanning a wrap reports the line the value starts on, and findings stay redacted.

  Existing behavior is untouched by construction: with nothing wrapped, the second
  pass sees exactly what the first did and every finding it produces is a duplicate
  that is dropped.
- **Withdraw now says when it could not verify ownership.** Withdraw is owner-only —
  engram compares the memory's recorded `owner` against your git email — but the check
  is skipped whenever either side is unknown, and it used to be skipped *silently*. A
  machine with no `user.email` configured could withdraw any memory, a teammate's
  included, with nothing on screen saying the guard had not run.

  The behavior is unchanged and deliberate: the guard still **fails open**, because
  refusing to withdraw your own memory because git is misconfigured would be the worse
  failure, and every team write already stops at a confirm. What changed is that the
  confirm now discloses it, and names *which* side is missing — the memory records no
  owner, or this machine has no git email — since one is the memory's history and the
  other is a single config line away. A verified owner, and a known mismatch (which
  withdraw refuses outright, as before), both render exactly as they did.

## [0.4.0] - 2026-08-21

Phase 4 tier 1: engram stops being Claude-only. The local instruction files the
other assistants read — `AGENTS.md`, `GEMINI.md`, GitHub Copilot's instructions
and Cursor rules — are now browsable read-only in `/files`, per project and, where
a vendor has one, from your home folder. A `scanRoots` setting lifts the limit that
a project had to have been opened in Claude Code before engram could see it. On the
team side, `>pull` learned to reconcile global-scoped memories with the same
direction-aware safety it already gave project ones.

### Added
- **Cursor rules and Copilot's path-scoped instructions are now in `/files`.** Every
  `.mdc` file under a project's `.cursor/rules/` (badged `cursor`) and every
  `.instructions.md` under `.github/instructions/` (badged `copilot`) lists read-only
  beside the project's other instruction files — folders inside the rules dir included,
  since Cursor documents organizing rules that way. This closes tier 1's last local
  format: the first sources that are *directories* of files rather than one file at a
  fixed path.

  These files carry frontmatter, and the preview treats it the way it deserves: the
  markdown body renders clean, and the one fact the frontmatter exists to state — which
  files the rules bind to — shows as a line under the title: `applies to src/**/*.ts`
  (from `globs` / `applyTo`), or `always applied`. A directory carrying only Cursor
  rules also counts as a project under `scanRoots` now.

  Deliberate boundaries, so nothing surprises later: only the documented extensions
  count (Cursor itself ignores a plain `.md` in its rules folder, so engram does too);
  nested `.cursor/rules` folders deeper inside a repo are not found (that would walk
  the whole repo tree on every 2s reload poll); the legacy `.cursorrules` file is
  skipped (current Cursor docs document `.mdc` rules and `AGENTS.md`, which engram
  already reads); and Cursor/Copilot *user-level* rules live in app settings, not
  files, so there is nothing to read.
- **The vendors' global instruction files are now in `/files`.** `~/.gemini/GEMINI.md`
  (Gemini CLI) and `~/.codex/AGENTS.md` (Codex) are listed in the `global` scope beside
  `~/.claude/CLAUDE.md`, with the same `gemini` / `agents` badges their per-project
  counterparts carry. This is the half of vendor-file support that was missing: engram
  showed a project's `GEMINI.md` but not the global one that applies to *every* project.

  Unlike per-project discovery, these are read straight from your home folder — so they
  appear with no `~/.claude` tree and no `scanRoots` configured. They belong to no project,
  so `@Claude` on one opens in `~/.claude` — the same place it already opened for the global
  `CLAUDE.md` — rather than in an unrelated repo.

  Two deliberate gaps: **Copilot** has no home-folder instructions file (VS Code keeps
  user-level instructions in profile settings), and Codex's `CODEX_HOME` relocation and
  `AGENTS.override.md` precedence are not honoured — engram reads the default
  `~/.codex/AGENTS.md` only.
- **Projects Claude Code has never opened can now appear, via configured scan roots.**
  A new `scanRoots` list in the config (`~/.config/engram/config.json`) names extra
  directories to look for projects in. Each root **and its immediate children** are
  checked, and a directory counts as a project only when it carries an instruction file
  (`CLAUDE.md`, `AGENTS.md`, `GEMINI.md` or `.github/copilot-instructions.md`). A leading
  `~` is expanded; a root must be absolute once expanded, and symlinked
  directories are not followed.

  ```json
  { "scanRoots": ["~/Documents/workspace", "~/work"] }
  ```

  This lifts the limit every tier-1 source inherited: engram previously reached a project
  only by decoding a folder key under `~/.claude/projects/`, so a repo you had never opened
  in Claude Code was invisible no matter which assistant's files it held.

  **Depth is deliberately 1.** The fingerprint that drives the reload poll re-runs this
  every 2 seconds, so a recursive walk would re-read a whole workspace tree tens of times a
  minute. One `ReadDir` per root plus a few stats stays negligible. Dotted directories are
  skipped, a project Claude already knows about is never listed twice (Claude's copy wins,
  since it is the one with a memory folder), and scan roots are **additive** — they still
  work when `~/.claude` is missing entirely. Scanned projects contribute instruction files
  only; they have no memory folder, so no `MEMORY.md` row.

  The config is re-read each poll tick on purpose, so adding a root takes effect live
  rather than at the next restart.

- **`GEMINI.md` and `.github/copilot-instructions.md` now show in the `/files` source** —
  Phase 4 tier 1 continues. Each project's Gemini CLI context file and repo-wide GitHub
  Copilot instructions are listed read-only alongside its `CLAUDE.md` and `AGENTS.md`,
  badged `gemini` and `copilot`, sorted `CLAUDE.md` → `AGENTS.md` → `GEMINI.md` →
  `copilot-instructions.md` → `MEMORY.md` within a project. External edits trigger the
  same poll reload as the other docs.

  The four vendor badges deliberately share one colour: the colour says "someone else's
  rules", the label says whose, so a new vendor costs a word rather than another hue in a
  small palette.

  **Two scope limits, stated plainly.** These inherit the `AGENTS.md` limit — engram finds
  them only for projects it already knows about, ones with a folder under
  `~/.claude/projects/`. And it reads each project's *own* file only: a vendor's global
  equivalent (`~/.gemini/GEMINI.md`) sits outside the `~/.claude` tree the walk is anchored
  to. The path-scoped rule *directories* — Copilot's `.github/instructions/*.instructions.md`
  and Cursor's `.cursor/rules/*.mdc` — are still out, because a directory of
  frontmatter-bearing files needs more than the flat `DocFile` shape.

  Internally the per-project instruction files moved to one `projectRuleFiles` table that
  discovery, the change fingerprint and the sort rank all read, so adding a vendor is one
  row instead of four edits that can drift apart.

- **`AGENTS.md` now shows in the `/files` source** — the first step of Phase 4 tier 1
  (other assistants). Each project's `AGENTS.md` is listed read-only next to its
  `CLAUDE.md`, carrying an `agents` badge so the two don't both read `rules`, and sorted
  `CLAUDE.md` → `AGENTS.md` → `MEMORY.md` within a project. `AGENTS.md` is the cross-tool
  instruction standard read by Codex, Cursor and Copilot (and by Claude Code itself).
  External edits trigger the same poll reload as the other docs.

  **Scope limit, stated plainly:** engram finds `AGENTS.md` only for projects it already
  knows about — ones with a folder under `~/.claude/projects/`. A repo you have never
  opened in Claude Code will not appear, because the walk starts from that folder. Lifting
  that needs a configured set of scanned roots, which is tracked separately.

  The site says so too: the roadmap card "Other assistants" moves from `next` to `started`,
  the FAQ answer changes from "Not yet" to "Partly" and names the scope limit outright, and
  `llms.txt` matches. The `FAQPage` schema was updated in lockstep with the visible card and
  re-checked word-for-word.

### Fixed
- **ROADMAP.md no longer misstates what shipped.** An audit of all 41 checkboxes
  against the code, git, the published cask and the live site found the code claims
  accurate but the file wrong in three places. Phase 3 stopped at `v0.2.1` and never
  learned about `v0.3.0` — while Phase 1 already said "shipped in `v0.3.0`", so the
  file contradicted itself. The release-tooling item claimed GoReleaser publishes the
  Homebrew cask with no caveat; that step worked for `v0.2.0` and `v0.2.1` but has
  returned 403 since `v0.3.0`, whose cask was applied by hand — both it and the tap
  item now say so. And the `GEMINI.md` item was ticked while the text two
  entries below said that item was not complete; the per-project scope it actually
  covers is now stated, and the global-file item no longer contradicts it.

  The `v0.3.0` docs sync updated `www/`, the derived files and README but not ROADMAP,
  which is how this survived.
- **The preview meta no longer repeats the project name.** A file sitting in a project's
  root has a parent directory whose name *is* the project, so the generic
  "project/parent/file" location line rendered `acme-site/acme-site/AGENTS.md`. It has read
  that way for every project's own `CLAUDE.md` since the `/files` source shipped; listing
  whole projects from a scan root is what finally made it obvious. Nested files keep their
  parent (`acme/.github/copilot-instructions.md`, `acme/memory/thing.md`).

### Changed
- **`>pull` now reconciles global-scoped memories too.** It previously walked the
  store's `projects/` folder only, so a global memory whose team copy had advanced
  showed `[behind]` but could only be taken through `>resolve` — a workaround, not a
  path. Pull now gives an existing local copy of a global memory the same
  direction-aware treatment as a project memory: fast-forward a clean incoming
  update, leave a local-ahead copy alone, flag a real divergence, never overwrite.
  The status bar and sync strip advertise `p pull` for a behind global memory
  accordingly, instead of steering into the resolve editor.

  One thing pull still never does: *place* a global memory you hold nowhere. It
  belongs to no project, so there is no folder to put it in — the store is its home
  until you promote it into a project yourself. Such memories are not counted as
  skipped; they are simply not local business yet.
- **The `/files` labels no longer name only `CLAUDE.md`.** The command palette entry read
  "CLAUDE.md & MEMORY.md — read-only" and the `@Claude` seed prompt said the user was
  browsing their "CLAUDE.md / MEMORY.md files" — both were already wrong once `AGENTS.md`
  landed and would have been wronger with four instruction files. They now read
  "instruction files & MEMORY.md — read-only" and "instruction and index files".
- **The hero pill above the headline now carries the release**, not a second copy of
  the pitch: `v0.4.0 out now · free and open source`, linking to the latest GitHub
  release. It previously read "your team's AI memory, in one place", which said the
  same thing as the headline directly under it and the subhead under that. The
  version is hardcoded like the ones in the terminal mockups, so cutting a release
  bumps all of them together.
- **The site moves from Netlify to Cloudflare Pages** (project `engram-im`) — same
  static `www/` upload, no build step, deployed with
  `npx wrangler pages deploy www --project-name=engram-im`. The DNS zone moved to
  Cloudflare too (Namecheap stays the registrar only): Pages can't verify an apex
  pointed by an ALIAS record, because that flattens to A records and verification
  wants a literal CNAME. Pages serves pages extensionless and 308-redirects
  `/privacy.html` → `/privacy`, so the footer link, `<link rel="canonical">`,
  `og:url`, `sitemap.xml` and `llms.txt` now all use `/privacy` — otherwise the
  canonical would point at a redirect. `netlify.toml` is removed.
- **The privacy policy names the right host.** It still credited Netlify with the
  site's server logs and linked Netlify's privacy policy; both now say Cloudflare.
  The hosting move updated this file's canonical and `og:url` but missed the two
  host references in the body.

### Security
- **Instruction files can no longer smuggle terminal escape sequences into your
  terminal.** engram renders files it does not own, and this release widened that
  considerably: `/files` now lists instruction files from any project under
  `scanRoots`, including repositories you have only cloned. Neither glamour nor
  lipgloss strips an escape sequence embedded in text it is handed, so a
  `.cursor/rules/*.mdc` whose `globs:` value carried an OSC sequence would have
  reached the terminal verbatim — rewriting the window title, or writing the
  clipboard where OSC 52 is permitted.

  Control characters are now stripped at the two chokepoints every rendered string
  passes through: `clip`, which covers all one-line metadata (titles, paths, badges
  and the new "applies to …" line), and the markdown body on its way *into* the
  renderer — never on the way out, since glamour's own output is ANSI by design.
  Newlines and tabs survive in a body because markdown uses both structurally.
  Stripping before measuring also corrects a width miscount, since an escape
  sequence occupies no display columns but its bytes were being counted as if
  they did.

### Removed
- **The project `.mcp.json` is gone.** It registered the `context7` and
  `sequential-thinking` servers at project scope, passing the context7 key as
  `${CONTEXT7_API_KEY}` — but Claude Code doesn't expand `${VAR}` in that file, so the
  server received the literal placeholder and rejected it as an invalid key. The config
  was version-controlled but non-functional for everyone. Contributors now register both
  servers themselves at user or local scope, with their own key; CLAUDE.md's review gate
  carries the commands. No secret was ever committed — the file held only the placeholder.

### Known issues
- **Email to `@engram.im` is silently dropped.** The domain's five `eforward*` MX
  records survive the nameserver move, but Namecheap's forwarding needs Namecheap's
  nameservers, so mail is accepted and discarded rather than bounced. No published
  address depends on it — `SECURITY.md`, `CODE_OF_CONDUCT.md` and the privacy page
  all use a direct address. Fix is to drop the MX records, or drop them and enable
  Cloudflare Email Routing.

## [0.3.0] - 2026-08-06

A design pass over the whole TUI — three themes on one token set, a two-row
header, a contextual status bar, one dialog anatomy — plus a confirm in front of
every team write, and a landing page and README screenshot brought up to match.

### Added
- **A sync strip in the preview** — a shared memory's preview now carries a band
  under the title: the state in a plain sentence ("The team copy moved ahead.
  Yours is untouched."), the offered action as a chip (`[p pull]`), and a
  direction gauge — `you ▬▬▬ ← ▬▬▬ team`, the side that moved filled in the
  state's color (conflict fills both) — with an honest timestamp. `edited` /
  `diverged` come from the local file; **`store advanced 2h ago` is read from
  the team store's git history** (fetched lazily for the selected row) and is
  simply omitted when git can't answer — never guessed. `missing` and `unknown`
  state their facts (`not in store`, `no anchor`) instead of claiming a time.
- **A pull shows its full accounting before anything moves** — `p` (or `>pull`)
  now fetches the store, runs the pull as a dry run (`team.PullPlan` — the
  identical walk with writes off), and opens a confirm listing exactly what
  will happen: what fast-forwards, what arrives new, what has local edits
  (left alone), what diverged (flagged, not overwritten), what you withdrew
  elsewhere (kept and marked personal), and what has no local project to land
  in. `y` applies that same walk with no second fetch, so the confirmed
  accounting is the applied accounting — every write the apply makes is a line
  in the dialog. A pull with nothing to write skips the dialog and says why.
- **A resolve shows the first conflict hunk before `$EDITOR` opens** — the
  git-style markers render in the confirm, so the decision is made before the
  handoff; cancelling removes the merge temp file and touches nothing.
- **Reconcile (`R`) confirms first, naming the actual files** — which files
  have no index line, which index lines point at no file, and what will be
  written; the toast then reports what happened (`2 index lines written to
  app/MEMORY.md`). A clean index answers `index already in sync` instead of
  silently rebuilding.
- **Landing-page search and AI-answer surface** (`www/`) — the site now ships
  `robots.txt` (nothing disallowed; AI answer agents such as `OAI-SearchBot`,
  `Claude-SearchBot` and `PerplexityBot` named explicitly, since they are separate
  user-agents from the training crawlers), `sitemap.xml`, and `llms.txt`. `index.html`
  gained `SoftwareApplication` and `FAQPage` structured data plus a visible **FAQ
  section** answering the six questions people actually ask — where Claude Code stores
  memories, how team sharing works, whether anything leaves the machine, how it differs
  from sharing `CLAUDE.md`, cost, and other assistants. The page title and
  description now lead with "Claude Code memory" rather than burying it, and
  `privacy.html` gained the Open Graph and Twitter card tags it was missing.
- **Landing-page CSS no longer scans the whole repo** (`www/css/input.css`) — Tailwind is
  now imported with `source(none)` and three explicit `@source` entries (the two pages
  plus `www/js/main.js`, which toggles class names as string literals). Previously the
  `@source` lines *added to* automatic detection rather than replacing it, so every Go
  file and Markdown doc was scanned and ordinary English words were harvested as class
  candidates — the shipped stylesheet carried junk utilities (`.grow`, `.table`,
  `.static`, `.visible`, `.h-1`, …). Removes 12 unused rules; no class used by the pages
  or by `main.js` changed.
- **README screenshot** (`docs/tui.png`) — a real capture of the TUI over staged,
  fictional demo data (an imaginary AI app). Regenerable after UI changes via the
  vhs tape and fixtures in `docs/demo/`; recaptured against the redesign, so the
  README shows the two-row header, the bare badges, and the padded status bar.

### Changed
- **Landing-page headings balance their lines, and body copy avoids widows** —
  the hero headline used to split `Your AI's memory, in` / `one terminal.`,
  filling the first line and orphaning the preposition away from "one terminal";
  it now reads as two even lines. `text-wrap: balance` on headings and
  `text-wrap: pretty` on prose are set once in `www/css/input.css` — inside
  `@layer base`, so a per-element Tailwind utility can still override them, and
  scoped past `.term` so the terminal mockups keep wrapping by character like the
  real TUI. Display headings also drop their trailing full stop, which the card
  headings already did.
- **The landing page no longer claims Claude syncs auto memory through Anthropic
  servers** — official Claude Code documentation says auto memory is
  machine-local. The comparison and FAQ now accurately distinguish
  repo-committed `CLAUDE.md` instructions from auto-learned memories and explain
  the gap engram fills with the user's own git remote.
- **Every dialog shares one anatomy with semantic colors** — an opaque panel
  framed in the dialog's meaning (delete/secret/resolve in danger,
  promote/withdraw/reconcile in warn, pull in info, new/palette/help in
  accent), an icon+title header, body copy with the lead line bright, and the
  actions bottom-right on a darker footer band. Copy follows the design spec:
  delete names the file and the MEMORY.md consequence, promote names both
  scopes and the `engram:` stamp, withdraw spells out the three consequences
  and the reassurance ("Promoting again puts it back"), and the secret dialog
  shows `line · rule` with the redacted match and an honest caveat — the
  spec's "override is recorded" claim is deliberately dropped, since engram
  keeps no audit log. Toasts name what happened (`memory deleted — MEMORY.md
  updated`, `promoted to the team store · pushed`, `withdrawn · tombstone
  pushed`, `merge written back — re-anchored as synced`).
- **Index drift warns from a banner above the list, not the title bar** — when
  the selected project's `MEMORY.md` is out of step with its files, a warning
  band takes the first list row: `△ app: 2 files have no line in MEMORY.md ·
  1 line has no file  [R reconcile]` — named project, both drift directions
  with real counts, one action. On narrower panes it sheds in a fixed order
  (project name first, then the chip — `R` stays offered in the status bar,
  whose hint now also reads `reconcile` instead of `fix index`) so the cause
  stays legible instead of drowning in an ellipsis. `esc` dismisses it for the
  session, per project, when no filter is active (a committed filter clears
  first); it never appears in the plans or files sources. The title bar's
  `⚠ index out of sync` pill is retired.
- **The palette is one sectioned list — no prefix required** — opening it
  (`ctrl+p` or `ctrl+k`) now shows everything at once: every memory and plan
  under **Jump to** (first result preselected), then **Sources**, **Team**, and
  **Assistant**, and typing filters all of it together — fuzzy over titles *and
  project names* for jumps, name prefixes for commands. The three-row prefix
  guide is retired; `/` `>` `@` survive as accelerators that scope the search
  (`>init <git-url>` works as before). Rows are single lines with the
  description right-aligned, and the `memory` command is now `memories` (the
  old spelling still resolves).
- **The status bar is contextual, with direct team keys** — `P` promote, `p`
  pull, and `r` resolve now work straight from the list (the `>` palette verbs
  remain), and the status bar leads with the one key that fits the selected
  row's sync state, in that state's color: a `[behind]` project row offers
  `p pull`, a `[behind]` global row offers `r resolve` (pull skips globals, so
  `p` would be dead there), and a personal row offers no team key at all. Keys
  render as keycaps, the `g` hint names the grouping you would switch *to*, and
  navigation hints moved to the `?` help. `@` now actually opens the selected
  project in Claude Code (it was advertised in the files view but unbound), and
  `ctrl+k` opens the palette alongside `ctrl+p` (inside the palette `ctrl+k`
  still moves up).
- **Sync states read as outlined word pills with the spec's vocabulary** — the
  list's filled sync pill becomes a bracketed word in the state's color
  (`[behind]`, bold on the selected row), and two states are renamed for
  clarity: **incoming → behind** and **differs → unknown** (display only; the
  `engram:` frontmatter and the Go API are unchanged). The preview's scope
  reads as a matching bracketed pill (`[global]`).
- **The preview header and list badges follow the prototype** — the title
  renders first with the meta line under it: the type badge as a bare colored
  word (list rows too — brackets dropped), a compact path
  (`app/memory/build-cache.md`), and the scope pill. The sync state word left
  the meta — the sync strip right below already states it in a sentence — and
  the `edited …` stamp stays only on personal rows, whose header has no strip
  to carry a timestamp.
- **A persistent source tab strip replaces the top-bar counts** — the chrome is
  now a two-row header block on the darker bar surface: first the source tabs,
  every source with its live count and the active tab as a filled block, with
  the brand and version tucked into the right corner; under it a controls row
  with the list-shaping chips — `type: all` `group: project`, labels dim,
  values bright, accent when a non-default shaping is active — and the palette
  hint (`^K jump or run anything`) always in its right corner. The `/ search`
  affordance renders as a keycap ("press `/` to search") and shows the
  committed query (`/ “q”`) while a search narrows the list; the filter input
  replaces the chips while typing. A rule row closes the header and doubles as
  the focus signal — the focused pane's side draws in accent. `shift+tab`
  cycles sources from either pane; the palette's `/memories` `/plans` `/files`
  commands still work. The status bar sits in a padded band and carries the
  theme switcher in its right corner (`theme Midnight · 1–3 switch · ? help`).
  Per-source cursor position, the type filter, and the group mode all persist
  across switches; a search still clears.
- **The TUI paints every cell with theme backgrounds** (`internal/tui/paint.go`) —
  list, preview, bars, rules, and dialogs each carry their theme surface color, so
  all three themes render identically regardless of the terminal's own palette
  (the prerequisite for the light Paperback theme being usable in a dark terminal
  and vice versa). Dialogs are now opaque panels over a **scrim**: while a dialog
  floats, the page behind it blends toward the surface color (truecolor terminals;
  elsewhere the opaque panel alone provides the separation). Dialog corners render
  near-square against the fill — a terminal cell can't clip a rounded corner over
  a background.
- **Three themes replace the previous five** (`internal/tui/theme.go`) — **Midnight**
  (Dracula-derived, the default), **Paperback** (light), and **CRT** (green phosphor),
  switched with `1`–`3`. Every color in the UI now flows through one named token set
  per theme (surfaces, text, and semantic `ok`/`info`/`warn`/`danger` colors); the
  previously fixed cross-theme sync-pill, scope-chip, and status colors are per-theme
  tokens, which is what makes a light theme possible at all. `config.json` now stores
  a stable lowercase key (`"midnight"` · `"paperback"` · `"crt"`); configs written by
  older versions still resolve (the five retired theme names map to Midnight), and
  older binaries reading the new keys fall back to their default theme rather than
  erroring.
- **Landing-page copy rewritten in a plainer voice** (`www/index.html`, `www/privacy.html`,
  `www/llms.txt`): every em dash is gone from user-visible site copy, and the sentences it
  was holding together are now periods, colons, or the `·` separator the page already uses.
  Two overclaims went with it, "your team in one keystroke" (promoting, pushing, and a
  teammate pulling is not one keystroke) and "the answer for repos that can't leave your
  own walls"; the `● differs` badge no longer explains itself as "no anchor to name a
  direction". The one remaining em dash is inside the palette mockup, where the string is
  copied verbatim from `internal/tui/palette.go` and must stay character-for-character
  identical to what the TUI renders.
- **The FAQ is now an accordion, one answer open at a time** (`www/index.html`,
  `www/js/main.js`, `www/css/input.css`): the six cards became native `<details>`/`<summary>`
  rows, so every answer is still readable with JavaScript off and stays in the DOM for the
  `FAQPage` schema; `main.js` only adds the "opening one closes the others" behavior. The
  first row is open by default. `summary::-webkit-details-marker` is hidden for Safari,
  which `list-none` alone does not cover.
- **The FAQ is reachable from the nav** (`www/index.html`) — the new section was linked
  only from the footer, so it was missing from the header nav and the mobile drawer, and
  the scroll-spy (which pins the *last* nav id active once the page bottom is reached)
  left "roadmap" highlighted while the reader was in the FAQ. The header nav tightens to
  `gap-4` until `lg` so five items still fit on one line at the `md` breakpoint.
- **Landing-page terminal mockups render the real TUI design** (`www/`): the
  browse / promote / pull demo, sync guide, and command palette mirror the
  current two-row source/control header, title-first preview, bare type labels,
  outlined sync states, preview sync strip, shared dialog anatomy, padded
  contextual footer, three-theme switcher, and sectioned prefix-optional
  palette. The `.term` tokens come directly from `internal/tui/theme.go`
  (Paperback in site light mode, Midnight in dark mode). Inactive stacked demo
  panels and the closed mobile drawer are inert and hidden from assistive
  technology. Small secondary text uses the stronger `Dim` token, and Paperback's
  OK/Warn-derived labels use web-only darker foregrounds, so 12–14px terminal text
  clears AA contrast without changing the app's source tokens; selected accent
  text gets the same web-only treatment in both themes. The browse and palette
  footers now match the TUI wording exactly.
- **The landing page degrades cleanly without JavaScript** — controls that cannot
  act (theme, demo/install tabs, copy buttons, and the mobile drawer trigger) are
  hidden, a direct mobile navigation row replaces the drawer, the browse demo
  remains visible, and the native FAQ accordion still works.
- **The landing page distinguishes "no engram service" from "no server"** —
  team sharing still uses the user's chosen git host. Promote copy now states
  that engram performs the commit and push rather than implying a separate
  manual push.
- **Phase 4 roadmap reframed in two honest tiers** (ROADMAP, SPEC §10, README, and
  the engram.im roadmap card): local instruction-file sources first (`AGENTS.md`,
  `GEMINI.md`, `.github/copilot-instructions.md`, Cursor rules — files on disk, no
  API needed), then server-side memories (Claude.ai / ChatGPT / Gemini app) via
  export/import, since none of them exposes a memory API today. The site's
  comparison table now says "Gemini app" to distinguish it from the Gemini CLI,
  whose `GEMINI.md` memory lives locally.
- **README no longer implies engram runs natively on Windows** — Windows binaries
  still ship with releases but are marked untested (two known Unix assumptions:
  project-path decoding resolves from `/`, and the editor fallback is `vi`); WSL
  is the supported route on Windows. Tracked in SPEC §10.
- **README restructured for readability** — status/website-build blockquotes replaced
  with a one-line status (build details now pointed at CONTRIBUTING.md), team sharing
  broken out per command, badge/palette explanations deduplicated into single homes,
  and previously undocumented surface documented: `engram version` / `help` CLI
  commands, the `theme` / `editor` config keys, the palette's bare-text fuzzy jump,
  and auto-reload on disk changes.

### Removed
- **`OPEN_QUESTIONS.md`** — a Phase 2 design scratchpad that nothing linked to and that
  the shipped implementation had overtaken (it still described retired badge names,
  multi-select promote as shipping first, and conflict resolution as opening two files).
  The questions still genuinely open, including monorepo sub-keys and alias coordination
  for remote-less projects, now live in SPEC §10 alongside the rest.

### Fixed
- **A pull whose only work is demoting your own withdrawn memory now runs.**
  When you withdraw a memory on one machine, your other checkouts still say
  `scope: team`; a pull resets them to personal, keeping the file. That demote
  was counted as no work at all, so a pull with nothing else pending reported
  "nothing to pull" and never applied it — the copy stayed `scope: team`
  indefinitely. It is now counted in the plan, named in the confirm dialog
  ("1 you withdrew elsewhere — kept here, marked personal"), and reported in
  the toast. It was also the one write the apply made without listing it in
  the accounting the user confirmed.
- **The pull confirm names skipped memories too** — team memories whose project
  has no local match were only mentioned when there was nothing else to do, so
  a pull with other work silently omitted them from its "full accounting".
- **`R` no longer claims a clean index when the check itself failed** — if the
  drift check couldn't read the memory dir, the empty result was reported as
  `index already in sync — nothing to write`, a statement about a check that
  never completed. It now says `could not check the index: …` instead.
- **The drift warning no longer disappears after visiting plans or files** —
  leaving the memories source cleared the drift flag but kept its cache key, so
  returning to the same project skipped the recompute and the warning stayed
  off until a reload. Present since the drift check shipped.
- **A theme switch now updates the preview pane on the first keypress** — the
  preview cache was refilled with the old theme's glamour rendering before the
  new renderer was built, so the right pane only caught up on a second press of
  `1`–`3`. Present since themes shipped, not introduced by the redesign.
- **Switching themes no longer wipes unrelated settings from `config.json`** — the
  save now round-trips the file, so `secretScanAction`/`secretScanScope` survive a
  `1`–`3` keypress. Previously every theme switch rewrote the file with only `theme`
  and `editor`.
- **A hand-edited config with an unknown theme keeps the current theme** on settings
  reload instead of resetting (the unknown value is also left in the file untouched).

## [0.2.1] - 2026-07-04

### Fixed
- **Withdraw no longer leaves a memory stuck as `! missing`.** `>withdraw` now
  verifies it can reset the local memory back to personal **before** removing the
  shared copy from the store. Previously, if that local rewrite failed — e.g. a memory
  whose frontmatter can't be safely edited — the store copy was already removed and
  pushed while the local copy stayed team-scoped, leaving it flagged `! missing`.
  Withdraw now refuses up front and leaves the memory fully shared and untouched.

## [0.2.0] - 2026-07-04

**Phase 2 — team sharing over git.** Share memories across a team through a plain
git repository you host — no server, no third party. Promote a memory to a shared
store, pull teammates' memories into their matching local projects, and resolve
conflicts in your editor — with direction-aware sync badges and a secret-scan guard
on the way out. Every team action lives under the `>` command palette (`ctrl+p`,
then `>`). See [ROADMAP.md](ROADMAP.md) and [SPEC.md](SPEC.md) §7.

### Added
- **`engram init-team <git-url>`** — one-time setup subcommand that clones the
  shared team repo to `~/.config/engram/team/` and, when the repo is empty,
  scaffolds `global/`, `projects/`, and `MEMORY.md`, then commits and pushes.
  A failed push is non-fatal (the local commit is kept, with a retry hint), and
  git's own output — auth prompts, progress, errors — is shown directly. Also
  available from inside the TUI as **`>init <git-url>`**.
- **Promote to team (`>promote`)** — promote the selected memory to the shared
  store: a scope dialog picks **this project** or **global**, then engram stamps the
  memory with an `engram:` frontmatter block (a durable id, scope, project, owner —
  preserving Claude's own keys), writes the copy under `global/` or
  `projects/<key>/`, and commits + pushes. A filename collision with a *different*
  memory is refused rather than overwritten.
- **Pull from team (`>pull`)** — fetch the team store and copy project-scoped team
  memories into their matching local projects (matched by git remote), refreshing
  each `MEMORY.md`. A clean incoming update **fast-forwards** automatically; a copy
  you edited is left alone; a genuine divergence stays a conflict to resolve. Pull
  never overwrites a differing local file, no-ops identical ones, and skips projects
  with no local match. Global-scoped team memories stay in the store (browse /
  promote-on-demand). The summary reports new / updated / ahead / withdrawn counts.
- **Withdraw from team (`>withdraw`)** — the reverse of promote: after a confirm, remove the
  selected memory's shared copy from the team store, reset it to personal (keeping
  its id), and commit + push. **Only the owner can withdraw** (an advisory
  guardrail). The removal is recorded in a `.engram-withdrawn` tombstone, so on a
  teammate's next **pull** their local team copy is removed too — a withdrawal
  *propagates*. Re-promoting clears the tombstone; personal files are never removed.
- **Sync-status badges** — team-scoped memories carry a compact glyph in the list
  showing their state against the team store, recomputed on launch and every reload.
  Backed by a **sync anchor** (a `syncedHash` recorded in the `engram:` block on
  every promote/pull — a digest of the shared content), engram names a *direction*
  when it can: `✓` synced, `↓` incoming (the store advanced, your copy is untouched —
  safe to take), `↑` ahead (you have unshared edits, the store is unchanged — promote
  to share), `↕` conflict (both moved), `!` missing (promoted, no copy in the store).
  Without an anchor (memories shared before the anchor existed) it falls back to a
  direction-less `●` differs — never a wrong direction claim. Personal memories carry
  no badge and the column disappears with no team store, so the feature stays
  invisible until you use team sharing. The preview spells the state out
  (`team global · incoming`).
- **Scope chip** — a color-coded `global` (teal) / `project` (azure) chip next to the
  sync pill shows which bucket a shared memory lives in. It's tied to the sync pill (never an orphan)
  and only appears for team-scoped memories once a store is set up.
- **Conflict resolution (`>resolve`)** — on a `↕ conflict` (or `● differs`, or an incoming
  global) memory, `>resolve` opens a git-style merge of both versions' shared content
  (`<<<<<<< yours … ======= … >>>>>>> team`) in `$EDITOR`; on save engram writes the
  resolved content back, re-anchored to the store version so taking theirs reads as
  synced and keeping a merge reads as ahead. Saving with markers still present, or
  emptying the file, aborts and leaves the memory untouched.
- **Secret-scan guard on promote** — before a memory is pushed to the shared store,
  engram scans it for credentials (AWS / GitHub / Anthropic / OpenAI / Stripe /
  Google / Slack keys, private-key blocks, JWTs, `scheme://user:pass@` URLs, and
  secret-named env vars regardless of framework prefix — `REACT_APP_`, `VITE_`,
  `NEXT_PUBLIC_`, `NUXT_`, …). By default a match blocks the promote with a modal listing the
  **redacted** findings and an informed override. Configurable via
  `secretScanAction` (`block` (default) / `block-strict` / `warn` / `off`) and
  `secretScanScope` (`secrets` (default) / `secrets+pii`). The curated set catches
  the common formats, not everything — a guard paired with the override, not a
  guarantee — and the raw secret is never displayed or logged.
- **`>` command palette for team actions** — promote / pull / withdraw / resolve, plus
  **`>init <git-url>`** to set up the store from inside the TUI, live under `ctrl+p` →
  `>` (a third prefix beside `/` sources and `@Claude`). Each acts on the selected
  memory, with the not-initialized guard centralized behind one message that points at
  `>init`. Team sharing shells out to `git` for everything; if it isn't on `PATH`,
  every `>` verb says so with an install link instead of a raw `executable file not
  found` error. (Local browsing needs no git.)
- Internal: `internal/team` (git-backed store: init / promote / pull / withdraw /
  resolve, plus read-only `SyncStates` and `ProjectKey`), `internal/secrets` (curated
  credential scanner), and a lossless `engram:` frontmatter round-trip in
  `internal/memory` (`ReadEngram`/`WriteEngram`, preserving Claude's own keys).

### Changed
- **Landing page (`www/`) rebuilt** with Tailwind CSS (stock theme only) compiled to a
  committed `www/css/styles.css` via `npm run build:css`. Consolidated to a shorter layout
  with an interactive terminal demo whose `browse` / `promote` / `pull` view tabs live in
  the terminal's title bar and auto-advance (keyboard-accessible), a dedicated
  command-palette section, light / dark / system themes, and accessibility passes
  (ARIA tabs, focus-visible rings, `prefers-reduced-motion`). Build tooling
  (`package.json`, `www/css/input.css`) added; `node_modules/` is gitignored.
- **Landing-page assets split into subfolders** — `www/css/` (Tailwind input + built
  output) and `www/js/main.js` (page behavior as a plain classic deferred script, no
  modules or dependencies). Only a tiny pre-paint theme guard stays inline in `<head>`;
  the copy buttons are wired via `addEventListener` instead of inline `onclick`.

### Fixed
- **List "ghost" cells eliminated.** Navigating the list left gray residual cells
  (col-0 blocks and full-width bands on spacer rows) that only a terminal resize
  cleared. Root cause: a glamour inline-code chip at the end of a wrapped preview
  line left its background open, so `clampFrame`'s padding — and the first cells of
  the next row — inherited it. `clampFrame` now closes every emitted line with a
  reset. Also bound the frame to the terminal (cap the preview pane to the panes
  height, stop inflating the viewport width past the pane, and clamp the frame to
  `height-1` lines) so a long preview or a short terminal can't over-scroll the
  alt-screen and desync Bubble Tea's renderer.
- **Selected row** keeps its highlight and now also shows an accent chevron + bold
  accent title; the unused `SelFg` theme field was removed.

### Security
- **Promote and pull refuse to act through a symlink in the team store.** A teammate
  with push access could commit a symlink pointing outside the store (at a shell rc,
  `~/.ssh`, …); promote would have written *through* it (an arbitrary-file overwrite)
  and pull would have read *through* it. Both now reject any store path that traverses
  a symlink, `init-team` clones with `core.symlinks=false` (persisted for later pulls),
  and submodule recursion is disabled.
- **`init-team` blocks git's `ext::` transport.** A pasted `ext::<cmd>` URL could run an
  arbitrary command at clone time; the clone and pull now set `protocol.ext.allow=never`
  regardless of the user's git config.
- **Withdrawal never deletes an edited memory.** Withdraw-propagation on pull now keeps
  a local team copy whose content no longer matches its sync anchor, so a withdrawal
  can't silently discard unshared local edits (the copy shows `! missing` instead).
- **The secret scan now catches PGP private-key blocks** (`-----BEGIN PGP PRIVATE KEY
  BLOCK-----`), which previously slipped through the promote guard.

### Known gaps
- **Global-scoped memories don't auto-pull yet** — `>pull` walks `projects/` only;
  an updated global memory is taken via `>resolve` (shown as `↓ incoming`).
- **Promote is single-select**; multi-select promote is pending.
- **No alias fallback** for projects without a git remote (they can't yet be keyed).
- Memories shared before the sync anchor existed show the direction-less `●` differs
  until their next promote/pull re-anchors them.

## [0.1.2] - 2026-06-25

A keybinding help overlay and a refreshed dialog style.

### Added
- **`?` — keybinding help overlay.** A floating cheat-sheet listing every key,
  grouped for readability, with an about footer (`version · engram.im · MIT`).
  Any key closes it; `?` is also shown in the bottom hint bar.

### Changed
- Restyle the floating dialogs (command palette, help, new-memory, delete
  confirm) as a flat rounded outline — smooth corners on the terminal
  background instead of a filled panel — with the selected/target row bleeding
  edge-to-edge to the border. The command-palette input now fills the dialog
  width.

### Fixed
- Dialog inputs no longer overflow the frame border: the command-palette and
  new-memory fields each reserved one cell too few for the text cursor.

## [0.1.1] - 2026-06-25

Memory-list polish.

### Changed
- Bold the selected row's title so the highlighted row stands out clearly, even
  on themes whose selection background sits close to the base background.
- Size the type-badge column to the widest badge currently listed (still capped
  at `[reference]`) instead of a fixed width, so short badges like `[user]` no
  longer leave a wide gap before the title in type-filtered and `/files` views.

## [0.1.0] - 2026-06-24

First release. Local memory **and** plan browsing (Phase 1), plus assisted
memory maintenance — `@Claude` and a read-only `/files` source (Phase 1.5).

### Added
- Discover memories across all `~/.claude/projects/*/memory/` folders.
- Parse both on-disk memory shapes: YAML frontmatter, and plain markdown whose
  metadata comes from the project's `MEMORY.md` index.
- Two-pane TUI: searchable memory list + markdown preview rendered with Glamour.
- Filter / search memories with `/`.
- Edit the selected memory in `$EDITOR` (`e`), with reload on return.
- Create a new memory (`n`) from a title prompt, seeded with a frontmatter
  template, then open it in `$EDITOR`.
- Delete the selected memory (`d`) with a `y`/`n` confirmation.
- Cycle a type filter (`t`): all → user → feedback → project → reference →
  unknown.
- Group the memory list by project, with a colored header per group showing the
  project name and memory count `(N)`.
- Show a type badge on each memory (`[user]`, `[feedback]`, `[project]`,
  `[reference]`, `[other]`) so its kind is visible at a glance.
- Color-code type badges (user=blue, feedback=orange, project=green,
  reference=purple, other=gray) and add typography: colored group headers,
  indented rows, a `❯` cursor on the selected row, and dimmed descriptions.
- Toggle grouping between by-project and by-type with `g`.
- Keep the project's `MEMORY.md` index in sync: `new`/`delete`/`edit` upsert and
  remove the corresponding index bullets, and `R` reconciles a folder's index
  against its files (drops dangling bullets, adds missing ones).
- Discover and browse plan-mode plans alongside memories — a multi-source
  browser with a command palette (`ctrl+p`) and floating dialogs.
- Group the plans list by recency (Today / This week / Older) with the same
  colored headers, counts, and row layout as the memory list.
- Live theme switching (`1`–`5`) with a themed multi-pane layout.
- Persist the chosen theme and `$EDITOR` under the XDG config dir
  (`~/.config/engram/`).
- Auto-reload the browser when the memory files change on disk, detected via a
  lightweight filesystem signature.
- `engram --version` / `--help` report the build version and usage.
- **`/files` source** — a third, read-only browser (palette: `/files`) for the
  files Claude *manages* rather than the ones you hand-write: the global
  `~/.claude/CLAUDE.md`, each project's `CLAUDE.md` (when its directory resolves
  on disk), and each project's `MEMORY.md` index. They're view-only — `e`/`d`
  surface a hint to edit via `@Claude` instead — and changes made externally (or
  by `@Claude`) are picked up by the poll.
- **`@Claude`** in the command palette — type `@` in the palette (`ctrl+p`) to
  launch an interactive [Claude Code](https://claude.com/claude-code) session,
  seeded with the selected project's memory/plan health (index drift, locations,
  and — when a project folder was renamed — the orphaned-memory situation). It
  repairs what the `R` reconcile can't (malformed frontmatter, broken
  `[[links]]`, stranded memories) and can create, rewrite, merge, or reorganize
  memories/plans on request. engram suspends during the session (the same handoff
  as `$EDITOR`) and reloads on exit. Requires the `claude` CLI on `PATH`; when
  it's missing the action shows a one-line hint instead of failing.
- **Command palette guide rows** — the palette (`ctrl+p`) opens to two guide
  rows, **`/`** (commands) and **`@`** (assistant), each with a short
  description, instead of immediately listing every command. Typing `/` reveals
  `/memory`, `/plans`, `/files`, `/settings`; typing `@` reveals `@Claude`.
  Selecting a guide row seeds its prefix, so it doubles as a shortcut.
- **Severity-colored footer messages** — transient footer messages are
  color-coded: warnings and deletions render as white on red, cancellations as
  dark brown on emerald.
- Landing page for **engram.im** (`www/index.html`) — at the time, a single self-contained
  HTML page in the Classic Dark theme (later rebuilt with a Tailwind build step + light/dark/
  system themes; see [0.2.0]).
  Served via Netlify from `www/` (see SPEC §9).
- Project `.mcp.json` registering the `context7` and `sequential-thinking` MCP
  servers so Claude Code (not just VSCode) can use them; the context7 key is read
  from the `${CONTEXT7_API_KEY}` environment variable, so no secret is committed.

### Changed
- The "index out of sync" warning names its cause — how many files were added
  without a `MEMORY.md` index line, and/or how many index entries point to a
  deleted/renamed file — instead of just flagging that drift exists.
- Internal: split the ~1.9k-line `internal/tui/tui.go` into focused same-package
  files (`model`, `update`, `view`, `items`, `palette`, `render`, `style`,
  `editor`, `status`, `layout`, `navigation`, `reload`); no behavior change.

### Fixed
- Project group names that contain dots (e.g. `engram.im`, `acme.dev`, or a
  domain-style folder like `app.engram.im`) now display in full. Claude flattens
  `/`, `.`, and `-` all to `-` when encoding a project folder, so decoding rebuilt
  `engram.im` as `engram/im` — showing the group as just `im`. Decoding now
  reconstructs the real path by matching folder names on disk, recovering dotted
  and multi-separator names. Affects both the memory and `/files` sources.

### Known gaps
- Team sharing over git (promote / pull, sync-status badges) is the next phase.

[Unreleased]: https://github.com/ertugrulhaskan/engram/compare/v0.4.0...HEAD
[0.4.0]: https://github.com/ertugrulhaskan/engram/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/ertugrulhaskan/engram/compare/v0.2.1...v0.3.0
[0.2.1]: https://github.com/ertugrulhaskan/engram/compare/v0.2.0...v0.2.1
[0.2.0]: https://github.com/ertugrulhaskan/engram/compare/v0.1.2...v0.2.0
[0.1.2]: https://github.com/ertugrulhaskan/engram/compare/v0.1.1...v0.1.2
[0.1.1]: https://github.com/ertugrulhaskan/engram/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/ertugrulhaskan/engram/releases/tag/v0.1.0
