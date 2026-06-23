// Command engram is a terminal UI for browsing your Claude Code memories.
package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ertugrulhaskan/engram/internal/config"
	"github.com/ertugrulhaskan/engram/internal/memory"
	"github.com/ertugrulhaskan/engram/internal/plan"
	"github.com/ertugrulhaskan/engram/internal/tui"
)

func main() {
	mems, err := memory.Discover("")
	if err != nil {
		fmt.Fprintln(os.Stderr, "engram: "+err.Error())
		os.Exit(1)
	}
	plans, err := plan.Discover("")
	if err != nil {
		fmt.Fprintln(os.Stderr, "engram: "+err.Error())
		os.Exit(1)
	}
	if len(mems) == 0 && len(plans) == 0 {
		fmt.Println("No Claude memories or plans found under ~/.claude/")
		return
	}

	p := tea.NewProgram(tui.New(mems, plans, config.Load()), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "engram: "+err.Error())
		os.Exit(1)
	}
}
