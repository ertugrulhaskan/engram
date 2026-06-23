package plan

import (
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// root resolves the plans directory, defaulting to ~/.claude/plans.
func root(dir string) (string, error) {
	if dir != "" {
		return dir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claude", "plans"), nil
}

// Discover returns all plans under dir (default ~/.claude/plans), newest first.
// A missing directory yields an empty list, not an error.
func Discover(dir string) ([]Plan, error) {
	d, err := root(dir)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(d)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var plans []Plan
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		path := filepath.Join(d, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		p := Plan{Title: titleOf(string(data), e.Name()), Body: string(data), Path: path}
		if info, err := e.Info(); err == nil {
			p.Modified = info.ModTime()
		}
		plans = append(plans, p)
	}
	sort.SliceStable(plans, func(i, j int) bool {
		return plans[i].Modified.After(plans[j].Modified) // newest first
	})
	return plans, nil
}

// Signature is a cheap fingerprint (name + modtime + size of every *.md) of the
// plans dir, for change-detection polling. Reads no file contents.
func Signature(dir string) (string, error) {
	d, err := root(dir)
	if err != nil {
		return "", err
	}
	entries, err := os.ReadDir(d)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	h := fnv.New64a()
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		fmt.Fprintf(h, "%s\x00%d\x00%d\n", e.Name(), info.ModTime().UnixNano(), info.Size())
	}
	return strconv.FormatUint(h.Sum64(), 16), nil
}

// Delete removes a plan file from disk.
func Delete(path string) error { return os.Remove(path) }

// titleOf derives a display title: the first "# " heading (with a leading
// "Plan:" prefix stripped), else the filename titleized.
func titleOf(body, filename string) string {
	for _, ln := range strings.Split(body, "\n") {
		s := strings.TrimSpace(ln)
		if s == "" {
			continue
		}
		if strings.HasPrefix(s, "# ") {
			t := strings.TrimSpace(strings.TrimPrefix(s, "# "))
			t = strings.TrimSpace(strings.TrimPrefix(t, "Plan:"))
			if t != "" {
				return t
			}
		}
		break // first non-blank line isn't a heading → use the filename
	}
	return titleize(strings.TrimSuffix(filename, ".md"))
}

func titleize(name string) string {
	parts := strings.FieldsFunc(name, func(r rune) bool { return r == '-' || r == '_' })
	for i, p := range parts {
		if p != "" {
			r := []rune(p) // index by rune so a multibyte leading char isn't split
			parts[i] = strings.ToUpper(string(r[0])) + string(r[1:])
		}
	}
	return strings.Join(parts, " ")
}
