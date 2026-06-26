package team

import "fmt"

// ProjectKey resolves a project directory's canonical team key: it reads the
// directory's `origin` remote and normalizes it (see NormalizeRemote). The key is
// how the same project is matched across machines regardless of local clone path.
// It returns an error when the directory has no usable git remote — the caller
// decides whether to fall back to a user-assigned alias.
func ProjectKey(projectDir string) (string, error) {
	if projectDir == "" {
		return "", fmt.Errorf("no project directory")
	}
	url, err := runGitCapture(projectDir, "remote", "get-url", "origin")
	if err != nil {
		return "", fmt.Errorf("no git origin remote in %s", projectDir)
	}
	return NormalizeRemote(url)
}
