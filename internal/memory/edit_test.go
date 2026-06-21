package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"Hello World":     "hello-world",
		"  Trim Me  ":     "trim-me",
		"API base URL!":   "api-base-url",
		"--weird__name--": "weird-name",
		"":                "",
	}
	for in, want := range cases {
		if got := Slugify(in); got != want {
			t.Errorf("Slugify(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCreateAndDelete(t *testing.T) {
	dir := t.TempDir()

	path, err := Create(dir, "My New Memory")
	if err != nil {
		t.Fatal(err)
	}
	if base := filepath.Base(path); base != "my-new-memory.md" {
		t.Errorf("filename = %q, want my-new-memory.md", base)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "name: my-new-memory") {
		t.Errorf("template missing frontmatter name:\n%s", data)
	}
	if !strings.Contains(string(data), "# My New Memory") {
		t.Errorf("template missing heading:\n%s", data)
	}

	// A second create with the same title must not collide.
	path2, err := Create(dir, "My New Memory")
	if err != nil {
		t.Fatal(err)
	}
	if base := filepath.Base(path2); base != "my-new-memory-2.md" {
		t.Errorf("second filename = %q, want my-new-memory-2.md", base)
	}

	if err := Delete(path); err != nil {
		t.Fatal(err)
	}
	if pathExists(path) {
		t.Errorf("file still exists after Delete")
	}
}
