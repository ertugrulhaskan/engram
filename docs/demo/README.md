# Regenerating the README screenshot

`../tui.png` is a real capture of the TUI over the staged, fictional demo data
in `setup.sh` (an imaginary AI app — every project and memory here is made up).
Re-run it after any UI change so the screenshot never drifts from the app.

Requires [vhs](https://github.com/charmbracelet/vhs) (`brew install vhs`).

```sh
cd docs/demo
go build -o engram ../..   # build the current TUI
bash setup.sh              # stage the fictional demo home
vhs tui.tape               # drive the TUI, write tui.png
cp tui.png ../tui.png      # promote the new capture
```

The tape selects the "RAG pipeline defaults" memory (`Down 4`); if `setup.sh`'s
fixtures change, recount the rows and adjust. Generated artifacts (`engram`,
`home/`, `src/`, `demo.gif`, `tui.png`) are gitignored.
