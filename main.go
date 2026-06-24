// Command engram is a terminal UI for browsing your Claude Code memories.
package main

import (
	"fmt"
	"os"
	"runtime/debug"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ertugrulhaskan/engram/internal/config"
	"github.com/ertugrulhaskan/engram/internal/memory"
	"github.com/ertugrulhaskan/engram/internal/plan"
	"github.com/ertugrulhaskan/engram/internal/tui"
)

// version is the release version, injected by GoReleaser via
// -ldflags "-X main.version=...". It's empty for any non-release build.
var version = ""

// appVersion resolves the version string for `engram --version`. A release
// binary has version set. For `go install module@vX.Y.Z` the version is empty
// but the build info carries the real tag, so we surface that. A plain
// `go build`/`go run` reports "(devel)" or a VCS pseudo-version (v0.0.0-…);
// those aren't meaningful releases, so we show "dev".
func appVersion() string {
	if version != "" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		if v := info.Main.Version; v != "" && v != "(devel)" && !strings.HasPrefix(v, "v0.0.0-") {
			return v
		}
	}
	return "dev"
}

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "version", "--version", "-v":
			fmt.Println("engram " + appVersion())
			return
		case "help", "--help", "-h":
			fmt.Println("engram — TUI for browsing your Claude Code memories; run with no args.")
			return
		default:
			fmt.Fprintln(os.Stderr, "engram: unknown argument: "+os.Args[1])
			os.Exit(2)
		}
	}

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
	docs, err := memory.DiscoverDocs("")
	if err != nil {
		fmt.Fprintln(os.Stderr, "engram: "+err.Error())
		os.Exit(1)
	}
	if len(mems) == 0 && len(plans) == 0 && len(docs) == 0 {
		fmt.Println("No Claude memories, plans, or docs found under ~/.claude/")
		return
	}

	p := tea.NewProgram(tui.New(mems, plans, docs, config.Load()), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "engram: "+err.Error())
		os.Exit(1)
	}
}
