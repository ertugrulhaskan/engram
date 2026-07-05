# Regenerates the README screenshot (../tui.png) — see README.md in this directory.
# $PWD expands at runtime inside the recorded shell, so the tape stays portable.
Output demo.gif

Set Shell "bash"
Set FontSize 20
Set Width 1600
Set Height 780
Set Padding 24
Set Theme "Dracula"

Hide
Type "HOME=$PWD/home XDG_CONFIG_HOME=$PWD/home/.config ./engram"
Enter
Sleep 3s
Show
Down@200ms 4
Sleep 2s
Screenshot tui.png
Sleep 500ms
Hide
Type "q"
Sleep 500ms
