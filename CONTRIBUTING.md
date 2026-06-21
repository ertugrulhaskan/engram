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

1. Open an issue describing the change (especially for anything in the v2/v3
   scope — check [ROADMAP.md](ROADMAP.md) first).
2. Fork, branch, and make your change with tests.
3. Ensure `go test ./...` and `go vet ./...` pass and the tree is `gofmt`-clean.
4. Open a pull request that explains the what and the why, and note anything you
   couldn't verify.

## Code of conduct

This project follows the [Contributor Covenant](CODE_OF_CONDUCT.md). By
participating you agree to uphold it.
