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
go build -ldflags "-s -w -X main.version=v0.3.0" -o engram ../..
bash setup.sh              # stage the fictional demo home
vhs tui.tape               # drive the TUI, write tui.png
cp tui.png ../tui.png      # promote the new capture
```

The tape selects the "RAG pipeline defaults" memory (`Down 4`); if `setup.sh`'s
fixtures change, recount the rows and adjust. Generated artifacts (`engram`,
`home/`, `src/`, `demo.gif`, `tui.png`) are gitignored.
