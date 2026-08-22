# Regenerating the README screenshot

`../tui.png` is a real capture of the TUI over the staged, fictional demo data
in `setup.sh` (an imaginary AI app — every project and memory here is made up).
Re-run it after any UI change so the screenshot never drifts from the app.

Requires [vhs](https://github.com/charmbracelet/vhs) (`brew install vhs`).

```sh
cd docs/demo
# Stamp the version you are shipping. A plain `go build` leaves it empty, and
# the header then shows a long VCS pseudo-version (v0.2.2-0.2026…) that gets
# clipped mid-string in the capture.
go build -ldflags "-s -w -X main.version=v0.4.0" -o engram ../..
bash setup.sh              # stage the fictional demo home
vhs tui.tape               # drive the TUI, write tui.png
cp tui.png ../tui.png      # promote the new capture
```

**Look at the PNG before promoting it.** Two things go wrong quietly, and both
were caught only by looking at the `v0.4.0` capture rather than trusting it:

- **vhs drops a glyph now and then.** One run lost the em dash from
  "(+11 pts) — not a bigger `top_k`" and the very next run rendered it. engram
  emitted the character both times — it is present in `View()` — so this is a
  ttyd/font painting flake, not an app bug. The fix is to eyeball the capture and
  re-run if a character is missing.
- **Capturing straight after `setup.sh` reads "edited just now"**, which is long
  enough to truncate to "edited just …" in the preview meta. Give the fixtures a
  few minutes and the stamp becomes "4m ago" and fits.

The tape selects the "RAG pipeline defaults" memory (`Down 4`); if `setup.sh`'s
fixtures change, recount the rows and adjust. Generated artifacts (`engram`,
`home/`, `src/`, `demo.gif`, `tui.png`) are gitignored.
