// Command engram is a terminal UI for browsing your Claude Code memories.
package main

import (
	"errors"
	"fmt"
	"os"
	"runtime/debug"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ertugrulhaskan/engram/internal/config"
	"github.com/ertugrulhaskan/engram/internal/memory"
	"github.com/ertugrulhaskan/engram/internal/plan"
	"github.com/ertugrulhaskan/engram/internal/team"
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
			fmt.Println("engram — TUI for browsing your Claude Code memories.\n\n" +
				"Usage:\n" +
				"  engram                       launch the TUI (no args)\n" +
				"  engram init-team <git-url>   clone & scaffold the shared team store\n" +
				"  engram version               print the version")
			return
		case "init-team":
			if len(os.Args) < 3 {
				fmt.Fprintln(os.Stderr, "usage: engram init-team <git-url>")
				os.Exit(2)
			}
			if err := team.InitTeam(os.Args[2]); err != nil {
				fmt.Fprintln(os.Stderr, "engram: "+err.Error())
				os.Exit(1)
			}
			dir, _ := team.Dir()
			fmt.Println("engram: team store ready at " + dir)
			return
		default:
			fmt.Fprintln(os.Stderr, "engram: unknown argument: "+os.Args[1])
			os.Exit(2)
		}
	}

	// Read, not Load: Load answers a broken config with defaults, which would
	// start the session with an empty projectAliases and promote an aliased
	// project's memories into global/ — a placement decision taken off a file
	// engram couldn't read. Every write path added alongside aliases refuses
	// that same file, so startup refuses it too. The error names the path and
	// the JSON fault, which is what fixing it needs.
	cfg, err := config.Read()
	switch {
	case errors.Is(err, config.ErrUnparseable):
		// Only a parse fault is fatal, and only because it is the one the user
		// can fix from the message: the error names the file and the JSON
		// fault. Any other read failure (a permission-denied config left
		// root-owned by one `sudo engram`, say) starts with defaults instead —
		// refusing to launch there would take away the TUI that repairs it and
		// leave no way in.
		fmt.Fprintln(os.Stderr, "engram: "+err.Error())
		os.Exit(1)
	case err != nil:
		fmt.Fprintln(os.Stderr, "engram: couldn't read the config, continuing with defaults: "+err.Error())
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
	docs, err := memory.DiscoverDocs("", cfg.ScanRoots)
	if err != nil {
		fmt.Fprintln(os.Stderr, "engram: "+err.Error())
		os.Exit(1)
	}
	if len(mems) == 0 && len(plans) == 0 && len(docs) == 0 {
		fmt.Println("No Claude memories, plans, or docs found under ~/.claude/")
		return
	}

	p := tea.NewProgram(tui.New(mems, plans, docs, cfg).WithVersion(appVersion()), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "engram: "+err.Error())
		os.Exit(1)
	}
}
