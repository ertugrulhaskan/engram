// Command engram is a terminal UI for browsing your Claude Code memories.
package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ertughaskan/engram/internal/memory"
	"github.com/ertughaskan/engram/internal/tui"
)

func main() {
	mems, err := memory.Discover("")
	if err != nil {
		fmt.Fprintln(os.Stderr, "engram: "+err.Error())
		os.Exit(1)
	}
	if len(mems) == 0 {
		fmt.Println("No Claude memories found under ~/.claude/projects/*/memory/")
		return
	}

	p := tea.NewProgram(tui.New(mems), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "engram: "+err.Error())
		os.Exit(1)
	}
}
