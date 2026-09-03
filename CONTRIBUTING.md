# Contributing to engram

Thanks for your interest! engram is a small, focused Go project, so getting
productive should take only a few minutes.

Please read [SPEC.md](SPEC.md) first — it's the source of truth for the design
and the decisions behind it.

## Prerequisites

- [Go](https://go.dev/dl/) 1.23 or newer
- `git`

## Build, run, test

```sh
git clone <repo-url> engram && cd engram
go mod tidy        # fetch dependencies (first time only, needs network)

go run .           # run it
go build -o engram # build a binary
go test ./...      # run the tests
go vet ./...       # static checks
gofmt -l .         # list any unformatted files (should print nothing)
```

The TUI needs a real terminal — run it in your own terminal, not through a pipe.

## Landing page (`www/`)

The site at [www/index.html](www/index.html) is styled with **Tailwind CSS, stock theme
plus one exception**: the terminal mockups use the `.term` token palette defined in
`www/css/input.css` (`t-*` utilities via `@theme inline`), whose values come verbatim
from `internal/tui/theme.go`: Paperback in site light mode and Midnight in site dark
mode. Keep those base hexes and semantic tokens in sync with `theme.go`; the
web-only text overrides strengthen Paperback's small OK/Warn-derived labels and
both themes' accent-on-selection text enough to meet web contrast requirements. Everything
*outside* the terminal mockups stays stock: no custom colors or breakpoints there; map
page colors to their nearest stock Tailwind shade. Character-cell widths in the mockups
(`w-[12ch]`-style arbitrary values) are allowed — they model terminal columns. The site
supports light / dark / system themes and is keyboard-accessible. Assets are split into
subfolders:

```
www/
    index.html       # markup + a tiny inline pre-paint theme guard in <head>
    privacy.html     # privacy policy
    robots.txt       # crawler policy — nothing disallowed, AI agents named explicitly
    sitemap.xml      # 2 URLs, hand-maintained <lastmod>
    llms.txt         # plain-text site summary for LLMs
    favicon.svg      # tab icon
    og.png           # Open Graph / Twitter card image
    css/
        input.css    # Tailwind entry — source(none) + one @source per scanned file
        styles.css   # generated, committed
    js/
        main.js      # page behavior — plain classic deferred script, no modules/deps
```

`robots.txt`, `sitemap.xml`, and `llms.txt` restate content that lives in `index.html`,
so they drift silently — when the page's pitch, install commands, or platform support
change, update them in the same commit (and bump the sitemap's `<lastmod>`). The two
`application/ld+json` blocks in `index.html` are subject to the same rule: the `FAQPage`
answers must stay **word-for-word identical** to the visible FAQ cards, since mismatched
FAQ markup is a Google structured-data policy violation. Keep `operatingSystem` in the
`SoftwareApplication` block at `"macOS, Linux"` until native Windows is actually verified
(SPEC §10) — it is a machine-readable support claim.

A note on `llms.txt`: it is included by maintainer preference, not evidence. Measurements
in 2026 found ~97% of `llms.txt` files are never fetched, AI crawlers read the HTML
directly, and no major vendor has committed to consuming it. Treat it as a cheap hedge,
and don't let it grow into a second source of truth.

```sh
npm install          # first time only — installs the Tailwind CLI (devDependency)
npm run build:css    # compile www/css/input.css -> www/css/styles.css (minified)
npm run watch:css    # rebuild on change while editing
```

`www/css/styles.css` is **committed** (Cloudflare Pages serves `www/` statically with no
build step), so rebuild and commit it whenever you change classes in `index.html` or
`privacy.html`.

Deploying the site is manual and separate from `git push`:

```sh
npx wrangler pages deploy www --project-name=engram-im --branch=main
```

Pages serves pages **extensionless** and 308-redirects `/privacy.html` → `/privacy`, so
internal links, `<link rel="canonical">`, `og:url`, `sitemap.xml` and `llms.txt` must all
use `/privacy`. Keep them consistent or the canonical points at a redirect.

**Sources are registered explicitly.** `input.css` imports Tailwind with
`source(none)`, so the only scanned files are the three `@source` lines: the two HTML
pages and `www/js/main.js`. Without `source(none)`, Tailwind's automatic detection also
scans the whole repo — every Go file and Markdown doc — and harvests ordinary English
words as class candidates, silently emitting junk utilities (`.grow`, `.table`,
`.static`, `.visible`…) into the shipped stylesheet; writing the word "grow" in a
sentence here was enough to change the CSS. **If you add a file that references Tailwind
classes, add an `@source` line for it** — `main.js` needs one because it toggles class
names as string literals (`bg-blue-600`, `dark:text-green-400`, `invisible`,
`translate-x-full`), and those exist nowhere in the HTML.
`www/js/main.js` is a plain classic script (no build step) — edit it directly. Keep the
pre-paint theme guard inline in `<head>` so the right theme paints on the first frame.
`node_modules/` is gitignored and must never be committed.

## Project layout & the one hard rule

```
main.go                  # entry point: discover → launch TUI; init-team subcommand
internal/
    memory/              # discovery + parsing + file mutation; the instruction-file (/files) source
    plan/                # plan-mode plans (a second read-only source)
    config/              # settings under the XDG config dir (theme, editor, scanRoots, projectAliases, secret scan)
    team/                # the shared team store over git
    secrets/             # credential scanning for the promote guard
    source/              # the per-source capability model — no UI and no IO
    tui/                 # Bubble Tea UI
```

**The layering rule:** everything outside `internal/tui` contains *no UI code*,
and `internal/tui` contains *no file logic*. The UI consumes parsed
`memory.Memory` values and calls into `memory` / `team` for anything that touches
disk or git; it never reads or writes files directly. Keep it that way — it's
what keeps the project testable. `SPEC.md` §8 lists every file and its job.

**What a source lets the user do is declared, not re-derived.** Each data package
states its own `source.Caps{Edit, Create, Delete, Share}` next to the code that has
to honour it, and the TUI gates every action on it. The zero value grants nothing,
so a new source is read-only until a capability is granted deliberately —
forgetting to declare one fails as a missing feature, never as a silent write. The
same rule governs any enum a dialog reads: make the *least* claiming value the zero
value, so a field nobody set can't decide what the UI offers.

## Guidelines

- **Format and vet** before committing: `gofmt -w .` and `go vet ./...`.
- **Add tests** for logic in the non-UI packages (they're pure and easy to test).
  See `internal/memory/*_test.go` for the style.
- **Never modify a user's memory files** except in response to an explicit user
  action (edit / create / delete / promote / withdraw / resolve). This is a core
  principle — see SPEC §2.
- **Stay compatible with Claude Code.** Only ever *add* optional frontmatter
  keys engram understands; don't rewrite Claude's fields.
- Keep commit messages clear and in the present tense ("add type filter", not
  "added type filter").

## Proposing changes

1. Open an issue describing the change (especially for anything in Phase 4 scope,
   or the Phase 2 refinements still listed as open — check
   [ROADMAP.md](ROADMAP.md) first).
2. Fork, branch, and make your change with tests.
3. Ensure `go test ./...` and `go vet ./...` pass and the tree is `gofmt`-clean.
4. Open a pull request that explains the what and the why, and note anything you
   couldn't verify.

## Releasing

Releases are automated with [GoReleaser](https://goreleaser.com) — see
[`.goreleaser.yaml`](.goreleaser.yaml) and
[`.github/workflows/release.yml`](.github/workflows/release.yml).

To cut a release:

1. Update [`CHANGELOG.md`](CHANGELOG.md): move `[Unreleased]` items into a new
   dated version section and refresh the compare/tag links.
2. Bump the hardcoded version strings on the site —
   `grep -nE 'v[0-9]+\.[0-9]+\.[0-9]+' www/index.html` finds them all: the hero release
   pill and the `engram vX.Y.Z` in each terminal mockup's header. Nothing generates
   these, so they only stay honest by hand. (Match the whole `vX.Y.Z` shape, not a
   `v0.`-prefixed one — a `v0.` pattern silently finds nothing the day the project
   reaches 1.0.0 and reads as "nothing to bump".)
3. Tag and push **the specific tag** (never `git push --tags` — that would also push
   any un-pushed local tags and fire stray releases):
   ```sh
   git tag -a vX.Y.Z -m "engram vX.Y.Z"
   git push origin vX.Y.Z
   ```
4. The `release` workflow first verifies that `HOMEBREW_TAP_TOKEN` can actually
   write the Homebrew tap, and **stops before publishing anything** if it cannot.
   GoReleaser creates the GitHub Release first and pushes the cask last, so without
   that gate a bad token yields a live release with a stale cask needing manual
   repair — which is what happened at `v0.3.0`. A failure here means no release was
   published; fix the token and re-push the tag.

   You can run that same check on its own at any time, without cutting a release —
   the **verify tap token** workflow (`gh workflow run verify-tap-token.yml`). A
   token's value can never be read back, so CI is the only place able to test the
   credential actually in use. Both workflows share
   `.github/scripts/verify-tap-token.sh`, so they cannot drift apart.

   The workflow then runs GoReleaser, which builds cross-platform binaries
   (macOS / Linux / Windows × amd64/arm64), creates the GitHub Release with
   archives + checksums, and pushes an updated formula to the Homebrew tap.
5. Deploy the site, so the version strings from step 2 actually reach visitors —
   deploys are manual and are *not* triggered by `git push` or by the release
   workflow:
   ```sh
   npx wrangler pages deploy www --project-name=engram-im --branch=main
   ```
   Until this runs, engram.im keeps advertising the previous version next to an
   install command that now installs the new one.

Validate config and do a full local dry-run (no publish) before tagging:

```sh
goreleaser check
goreleaser release --snapshot --clean --skip=publish   # builds into ./dist
```

**Tap + token (already configured):** the `ertugrulhaskan/homebrew-tap` repo exists
and the engram repo carries a `HOMEBREW_TAP_TOKEN` Actions secret with write access
to it (the built-in `GITHUB_TOKEN` can't push to a separate repository). That secret
should stay a **fine-grained PAT limited to the `homebrew-tap` repository** (contents:
read/write) rather than a broad account token. Fine-grained PATs expire, so if a release
builds cleanly but the Homebrew cask step fails with a `401`, an expired or rotated
token is the first thing to check.

## Code of conduct

This project follows the [Contributor Covenant](CODE_OF_CONDUCT.md). By
participating you agree to uphold it.
