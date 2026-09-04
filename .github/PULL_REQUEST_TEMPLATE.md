## What & why

Brief description of the change and the problem it solves. Link any related issue.

## Checklist

- [ ] `gofmt -w .`, `go vet ./...`, and `go test ./...` all pass
- [ ] Commit messages use conventional prefixes, present tense ("add x", not "added x")
- [ ] Layering respected: every package outside `internal/tui` (`memory`, `plan`, `config`,
      `team`, `secrets`, `source`) contains no UI; `internal/tui` contains no file/IO logic
- [ ] No user memory files modified except on an explicit user action
      (edit/create/delete/promote/withdraw)
- [ ] Docs updated in the *same* change where behavior, structure, or status
      changed — CHANGELOG, ROADMAP, SPEC, README as applicable (see [CLAUDE.md](../CLAUDE.md))
