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
only** — no custom colors, arbitrary values (`[...]`), or custom breakpoints; map any
color to its nearest stock Tailwind shade. It supports light / dark / system themes and
is keyboard-accessible. Assets are split into subfolders:

```
www/
    index.html       # markup + a tiny inline pre-paint theme guard in <head>
    css/
        input.css    # Tailwind entry (@source "../index.html")
        styles.css   # generated, committed
    js/
        main.js      # page behavior — plain classic deferred script, no modules/deps
```

```sh
npm install          # first time only — installs the Tailwind CLI (devDependency)
npm run build:css    # compile www/css/input.css -> www/css/styles.css (minified)
npm run watch:css    # rebuild on change while editing
```

`www/css/styles.css` is **committed** (Cloudflare Pages serves `www/` statically with no
build command), so rebuild and commit it whenever you change classes in `index.html`.
`www/js/main.js` is a plain classic script (no build step) — edit it directly. Keep the
pre-paint theme guard inline in `<head>` so the right theme paints on the first frame.
`node_modules/` is gitignored and must never be committed.

## Project layout & the one hard rule

```
main.go                  # entry point: discover → launch TUI
internal/
    memory/              # discovery + parsing + file mutation
    tui/                 # Bubble Tea UI
```

**The layering rule:** `internal/memory` contains *no UI code*, and
`internal/tui` contains *no file logic*. The UI consumes parsed `memory.Memory`
values and calls `memory.Create` / `memory.Delete`; it never reads or writes
files directly. Keep it that way — it's what keeps the project testable.

## Guidelines

- **Format and vet** before committing: `gofmt -w .` and `go vet ./...`.
- **Add tests** for logic in `internal/memory` (it's pure and easy to test). See
  `internal/memory/*_test.go` for the style.
- **Never modify a user's memory files** except in response to an explicit user
  action (edit / create / delete / promote). This is a core principle — see
  SPEC §2.
- **Stay compatible with Claude Code.** Only ever *add* optional frontmatter
  keys engram understands; don't rewrite Claude's fields.
- Keep commit messages clear and in the present tense ("add type filter", not
  "added type filter").

## Proposing changes

1. Open an issue describing the change (especially for anything in Phase 2/3
   scope — check [ROADMAP.md](ROADMAP.md) first).
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
2. Tag and push:
   ```sh
   git tag v0.2.0
   git push --tags
   ```
3. The `release` workflow runs GoReleaser, which builds cross-platform binaries
   (macOS / Linux / Windows × amd64/arm64), creates the GitHub Release with
   archives + checksums, and pushes an updated formula to the Homebrew tap.

Validate config and do a full local dry-run (no publish) before tagging:

```sh
goreleaser check
goreleaser release --snapshot --clean --skip=publish   # builds into ./dist
```

**One-time setup** (before the first public release): create the
`ertugrulhaskan/homebrew-tap` repo and add a `HOMEBREW_TAP_TOKEN` Actions secret
— a PAT with write access to that tap repo (the built-in `GITHUB_TOKEN` can't
push to a separate repository).

## Code of conduct

This project follows the [Contributor Covenant](CODE_OF_CONDUCT.md). By
participating you agree to uphold it.
