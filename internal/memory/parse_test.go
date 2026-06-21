package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDiscoverBothShapes verifies that Discover handles both on-disk memory
// shapes: YAML frontmatter, and plain markdown whose metadata lives in MEMORY.md.
func TestDiscoverBothShapes(t *testing.T) {
	root := t.TempDir()
	memDir := filepath.Join(root, "-tmp-proj", "memory")
	if err := os.MkdirAll(memDir, 0o755); err != nil {
		t.Fatal(err)
	}

	writeFile(t, filepath.Join(memDir, "dev-server.md"), `---
name: dev-server
description: dev server runs on :3000
metadata:
  type: project
---
# Dev server

It is already running.
`)
	writeFile(t, filepath.Join(memDir, "prefs.md"), `# User preferences

Likes tabs over spaces.
`)
	writeFile(t, filepath.Join(memDir, "MEMORY.md"), `# Memory Index

- [User preferences](prefs.md) — likes tabs; detail-oriented
`)

	mems, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(mems) != 2 {
		t.Fatalf("want 2 memories, got %d", len(mems))
	}

	by := map[string]Memory{}
	for _, m := range mems {
		by[m.Name] = m
	}

	// Frontmatter shape.
	fm, ok := by["dev-server"]
	if !ok {
		t.Fatal("dev-server not found")
	}
	if fm.Title != "Dev server" {
		t.Errorf("frontmatter title = %q, want %q", fm.Title, "Dev server")
	}
	if fm.Description != "dev server runs on :3000" {
		t.Errorf("frontmatter description = %q", fm.Description)
	}
	if fm.Type != TypeProject {
		t.Errorf("frontmatter type = %q, want %q", fm.Type, TypeProject)
	}
	if !strings.Contains(fm.Body, "already running") {
		t.Errorf("frontmatter body missing content: %q", fm.Body)
	}

	// Plain-markdown shape: description must come from the MEMORY.md index.
	pf, ok := by["prefs"]
	if !ok {
		t.Fatal("prefs not found")
	}
	if pf.Title != "User preferences" {
		t.Errorf("plain title = %q, want %q", pf.Title, "User preferences")
	}
	if pf.Description != "likes tabs; detail-oriented" {
		t.Errorf("plain description = %q (should come from MEMORY.md)", pf.Description)
	}
	if pf.Type != TypeUnknown {
		t.Errorf("plain type = %q, want %q", pf.Type, TypeUnknown)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
