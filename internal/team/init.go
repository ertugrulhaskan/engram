package team

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// InitTeam sets up the shared team store: it clones gitURL into the managed team
// directory (see Dir) and, when the cloned repo is empty, scaffolds the expected
// layout and publishes it. This is the one-time setup behind `engram init-team`.
//
// Cloning into an already-initialized (non-empty) team directory is refused. git's
// own output — auth prompts, progress, errors — is wired straight to the process so
// the user sees it. The final push is best-effort: a missing remote or credentials
// leaves the local commit intact with guidance rather than failing the command. If
// a step after cloning fails, a directory we created is removed so a retry is clean.
func InitTeam(gitURL string) error {
	dir, err := Dir()
	if err != nil {
		return err
	}
	if entries, err := os.ReadDir(dir); err == nil && len(entries) > 0 {
		return fmt.Errorf("team repo already initialized at %s", dir)
	}
	// Whether dir already existed (empty) before this call. Only a directory we
	// create here is cleaned up on failure — never one the user already had.
	preexisting := false
	if _, err := os.Stat(dir); err == nil {
		preexisting = true
	}
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return err
	}
	fail := func(e error) error {
		if !preexisting {
			os.RemoveAll(dir)
		}
		return e
	}

	// `--` ends git option parsing so a remote URL beginning with '-' can't be
	// mistaken for a flag.
	if err := runGit("", "clone", "--", gitURL, dir); err != nil {
		return fail(fmt.Errorf("git clone failed: %v", err))
	}

	// An empty (commit-less) clone gets the starter layout; a populated one is
	// left exactly as cloned.
	out, err := exec.Command("git", "-C", dir, "rev-list", "-n1", "--all").Output()
	if err != nil {
		return fail(fmt.Errorf("inspecting cloned repo failed: %v", err))
	}
	if strings.TrimSpace(string(out)) != "" {
		return nil // already has commits — nothing to scaffold
	}

	if err := scaffold(dir); err != nil {
		return fail(err)
	}
	if err := runGit(dir, "add", "-A"); err != nil {
		return fail(fmt.Errorf("staging scaffold failed: %v", err))
	}
	if err := runGit(dir, "commit", "-m", "Initialize engram team store"); err != nil {
		return fail(fmt.Errorf("committing scaffold failed: %v", err))
	}
	// Publishing the scaffold is the point of init-team, but a missing remote or
	// credentials must not discard the local commit — surface a retry hint instead.
	if err := runGit(dir, "push", "-u", "origin", "HEAD"); err != nil {
		fmt.Fprintf(os.Stderr,
			"engram: scaffold committed locally, but the push failed; run `git -C %s push -u origin HEAD` to publish\n", dir)
	}
	return nil
}

// scaffold writes the starter team-store layout into an empty clone: placeholder
// global/ and projects/ directories (each kept by a .gitkeep, since git won't
// track bare empty dirs) and a MEMORY.md header.
func scaffold(dir string) error {
	for _, keep := range []string{
		filepath.Join(dir, "global", ".gitkeep"),
		filepath.Join(dir, "projects", ".gitkeep"),
	} {
		if err := os.MkdirAll(filepath.Dir(keep), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(keep, nil, 0o644); err != nil {
			return err
		}
	}
	header := "# Team memories\n\nShared Claude Code memories for this team, managed by engram.\n"
	return os.WriteFile(filepath.Join(dir, "MEMORY.md"), []byte(header), 0o644)
}

// runGit runs a git subcommand with stdout/stderr wired to the process so git's
// output reaches the user directly. A non-empty dir runs the command inside that
// repo via `git -C dir`.
func runGit(dir string, args ...string) error {
	if dir != "" {
		args = append([]string{"-C", dir}, args...)
	}
	cmd := exec.Command("git", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// runGitCapture runs a git subcommand and returns its trimmed stdout. Use it for
// read-only queries engram needs to inspect (remote get-url, status, rev-parse);
// runGit is for user-facing operations whose output should stream to the terminal.
func runGitCapture(dir string, args ...string) (string, error) {
	if dir != "" {
		args = append([]string{"-C", dir}, args...)
	}
	out, err := exec.Command("git", args...).Output()
	return strings.TrimSpace(string(out)), err
}
