# Regenerating the README screenshot

`../tui.png` is a real capture of the TUI over the staged, fictional demo data
in `setup.sh` — an imaginary AI product ("nimbus"), its eval harness, and the
memories, plans and instruction files a team would actually accumulate around
them. Every project, path and dashboard name is made up. Re-run it after any UI
change so the screenshot never drifts from the app.

Requires [vhs](https://github.com/charmbracelet/vhs) (`brew install vhs`).

```sh
cd docs/demo
# Stamp the version you are shipping. A plain `go build` leaves it empty, and
# the header then shows a long VCS pseudo-version (v0.2.2-0.2026…) that gets
# clipped mid-string in the capture.
go build -ldflags "-s -w -X main.version=v0.5.1" -o engram ../..
bash setup.sh              # stage the fictional demo home
find home src -name "*.md" -exec touch -A -000400 {} +   # see the stamp note below
vhs tui.tape               # drive the TUI, write tui.png
cp tui.png ../tui.png      # promote the new capture
```

**Look at the PNG before promoting it.** Four things go wrong quietly, and each
was caught only by looking at a capture rather than trusting it:

- **vhs drops a glyph now and then.** A `v0.4.0` run lost the em dash from a
  fixture line that the very next run rendered fine. engram emitted the character
  both times — it is present in `View()` — so this is a ttyd/font painting flake,
  not an app bug. The fix is to eyeball the capture and re-run if a character is
  missing.
- **Capturing straight after `setup.sh` reads "edited just now"**, which is long
  enough to truncate to "edited just …" in the preview meta. `setup.sh` writes the
  fixtures at the current time and nothing backdates them, so the `touch -A -000400`
  above winds every fixture back four minutes and the stamp reads "4m ago" on the
  first run. Waiting works too, but the stamp then depends on how long you waited;
  backdating makes consecutive captures differ only where the app does.
- **A title longer than ~33 characters truncates in the list pane**, and a
  preview line longer than ~62 characters wraps. Neither number is a property of
  the app: both fall out of `tui.tape`'s `Set Width 1600` and `Set FontSize 20`,
  so changing either moves them and the fixtures need re-measuring. Both renders
  are honest, but a screenshot full of "…" sells the app short, so the memory
  fixtures are written to fit. Widening the tape instead would shrink the text in
  the README.
- **Every tab in the header is part of the screenshot.** `setup.sh` stages plans
  and instruction files as well as memories precisely so `plans` and `files` do
  not read `0` — an empty counter looks like a feature that does not work.
- **The project rule files live outside the fake home**, in `src/<project>/`, and
  engram reaches them by decoding each project key back to a real path. `src/` is
  gitignored and disposable, so a `git clean -xdf` or a fresh checkout leaves the
  three project `CLAUDE.md` files unreadable and `files` silently drops from 7 to
  4. Always re-run `setup.sh` immediately before capturing.

The tape selects the "RAG pipeline defaults" memory (`Down 4`). **`Down` steps
between memory rows only** — project headers and the blank row between groups are
skipped — so count memories, not screen lines. Within a project, **memories sort
by type (project, feedback, user, reference) and then alphabetically by title**;
`MEMORY.md` sets no order at all, and is read only to supply a title or hook that
a file itself is missing. So renaming a memory's heading, or changing its
`metadata.type`, re-anchors the tape silently. If `setup.sh`'s fixtures change,
recount and adjust.
Generated artifacts (`engram`, `home/`, `src/`, `demo.gif`, `tui.png`) are
gitignored, which is also what keeps the real local paths encoded in
`home/.claude/projects/` out of the repo.
