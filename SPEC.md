# engram — Specification

This document is the source of truth for what engram is, how it's built, and the
decisions behind it. It's written for contributors. For user-facing docs see
[README.md](README.md); for sequencing see [ROADMAP.md](ROADMAP.md).

---

## 1. Goal

A fast, open-source **terminal UI** for browsing, searching, and editing the
memories Claude Code keeps on disk — across all projects — and for **sharing**
the useful ones with a team over any git host.

Non-goal: replacing `CLAUDE.md`-in-a-repo. engram covers what that can't —
cross-project memories, personal-vs-team layering, and a real UI.

## 2. Principles

1. **Read-only by default.** Never modify a memory file unless the user
   explicitly edits or promotes it.
2. **Compatible with Claude Code.** Files stay readable by Claude. engram only
   ever *adds* optional frontmatter it understands; it never rewrites Claude's
   fields.
3. **No servers.** Sharing rides on plain git, so the project stays free to run
   and host-agnostic (GitHub / GitLab / Bitbucket / self-hosted).
4. **Single binary.** Ships as one downloadable file. Go makes this free.
5. **Small, layered code.** File logic and UI never mix.

## 3. Tech stack

| Concern            | Choice                                            |
|--------------------|---------------------------------------------------|
| Language           | Go 1.23+                                           |
| TUI framework      | Bubble Tea (Charm)                                |
| List / viewport    | Bubbles (Charm)                                   |
| Styling            | Lip Gloss (Charm)                                 |
| Markdown rendering | Glamour (Charm)                                   |
| Frontmatter        | `gopkg.in/yaml.v3`                                |
| Sharing transport  | git (shelled out), any host                       |
| License            | MIT                                               |

## 4. Where memories live

Claude Code stores memories per-project:

```
~/.claude/projects/<encoded-project-path>/memory/
    MEMORY.md          # human index of the folder
    <slug>.md          # one memory per file
```

`<encoded-project-path>` is the project's absolute path with `/` replaced by `-`
and a leading `-`, e.g. `/Users/me/code/app` → `-Users-me-code-app`.

> ⚠️ **Decoding is lossy.** Path segments can themselves contain `-`
> (`acme-site`, `work-bigco`), so `-`→`/` is ambiguous. engram
> decodes by probing the filesystem (slash first, then dash) and falls back to
> the raw encoded string as a stable key when decoding fails. The raw encoded
> name is always a valid project identity even when the pretty path isn't.

## 5. The two on-disk memory shapes

engram must parse **both**, because real installs contain both:

**A. YAML frontmatter** (the documented format):
```markdown
---
name: dev-server-already-running
description: the dev server is usually already up on :3000
metadata:
  type: project
---
Body in markdown…
```

**B. Plain markdown** (older / hand-written), with metadata in `MEMORY.md`:
```markdown
# User preferences
Body in markdown…
```
```markdown
<!-- MEMORY.md -->
- [User preferences](user-preferences.md) — highly detail-oriented on UI/design
```

### Parsing rules (precedence)

For each `*.md` in a memory dir (excluding `MEMORY.md`):

| Field         | Source order                                                        |
|---------------|---------------------------------------------------------------------|
| `Name`        | frontmatter `name` → filename without `.md`                         |
| `Title`       | first `# ` heading in body → `MEMORY.md` link title → titleized name |
| `Description` | frontmatter `description` → `MEMORY.md` hook → first body paragraph |
| `Type`        | frontmatter `metadata.type` → `unknown`                             |
| `Body`        | file content minus frontmatter                                      |

`MEMORY.md` index lines are parsed with:
`- [Title](file.md) — hook`  (em-dash, en-dash, or hyphen separators accepted).

## 6. Data model

```go
type Memory struct {
    Name        string   // slug
    Title       string   // human title
    Description string   // one-line hook
    Type        Type     // user | feedback | project | reference | unknown
    Body        string   // markdown body (no frontmatter)
    Raw         string   // full original file contents
    Path        string   // absolute path on disk
    Modified    time.Time // file modification time (drives recency + auto-reload)
    Project     Project
}

type Project struct {
    Name      string // friendly (basename of decoded path)
    Dir       string // decoded absolute project dir (best-effort)
    MemoryDir string // .../memory
    Remote    string // git remote URL — v2, empty in v1
}
```

## 7. Sharing design (v2)

The shared store is **one git repo** the whole team can read/write. engram keeps
a managed local clone and shells out to git for all sync.

### Project identity across machines

A teammate's local project path differs from yours, so **project-specific
memories are keyed by git remote URL**, not local path. engram reads the remote
with `git -C <project-dir> remote get-url origin`. Projects with no remote fall
back to a user-assigned alias.

### Shared repo layout

```
team-memory/
    global/<slug>.md            # apply everywhere
    projects/<remote-slug>/<slug>.md   # apply to one project
    MEMORY.md
```

### engram-only frontmatter

Added to files engram manages; Claude Code ignores unknown keys:

```yaml
engram:
  scope: team                       # personal | team
  project: bitbucket.org/acme/app   # git remote, or "global"
  owner: you@acme.com
```

### Operations

- **promote** `<one|many>` — copy selected personal memories into the clone, set
  `engram.scope: team`, commit, push. Multi-select supported.
- **pull** — `git pull` the clone, then place team files where Claude reads them
  (matching project, or global) and refresh the relevant `MEMORY.md`.
- Personal memories never leave the machine unless promoted.

### Sync-status (shown as badges in the list)

Every memory has a state relative to the team repo:

| Badge        | Meaning                              | Suggested action |
|--------------|--------------------------------------|------------------|
| `[personal]` | local only, intentionally private    | —                |
| `[+] new`    | local, not yet in the team repo      | promote          |
| `[team ✓]`   | local matches team                   | —                |
| `[team ●]`   | edited locally since promoting       | promote (update) |
| `[team ↓]`   | team has a newer version             | pull             |
| `[team ⚠]`   | both changed                         | resolve          |

## 8. Module layout

```
engram/
    main.go                  # entry point: discover memories + plans → launch TUI; --version/--help
    internal/
        memory/              # NO UI here
            memory.go        # types
            discover.go      # walk projects, decode paths, fs signature
            parse.go         # frontmatter + index parsing, fallbacks
            index.go         # MEMORY.md index upsert / remove / reconcile
            edit.go          # create / delete / open-in-$EDITOR
        plan/                # discover plan-mode plans under ~/.claude/plans (a second read-only source)
        config/              # load/save theme + editor under the XDG config dir
        tui/                 # NO file logic here
            tui.go           # Bubble Tea model/update/view; multi-source browser + command palette
            theme.go         # themes, colors, group coloring
            overlay.go       # floating dialogs
```

> v1 also browses **plan-mode plans** as a second source (read-only, grouped by
> recency), switchable via the command palette. The sharing design below (v2)
> concerns memories only.

## 9. Distribution

- `go install` for Go users.
- Tagged releases build cross-platform binaries (GitHub Releases / equivalent).
- Homebrew tap from the first release.

## 10. Open questions / future

- Conflict resolution UX for `[team ⚠]` (v2): inline diff vs. open both.
- Promoting whole *types* at once (e.g. "all feedback").
- v3: ingesting Claude.ai / ChatGPT / Gemini memories — blocked on those
  products exposing programmatic access; likely export/import at first.
