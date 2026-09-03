package team

import (
	"fmt"
	"sort"
	"strings"
)

// aliasPrefix is the store namespace for alias-derived keys: a project keyed
// by an alias lands under projects/alias/<name>/, beside the remote-derived
// host/path keys. The prefix makes the key's origin legible in the store tree
// and keeps a later shared alias map (SPEC §10) a plain name → key lookup.
// A remote whose host is literally "alias" (an ssh_config Host of that name)
// would normalize into this same namespace, so NormalizeRemote refuses that
// host outright rather than letting the two kinds of key share a bucket. That
// keeps IsAliasKey exact — it answers from the prefix alone, with no remote
// able to spell its way in.
const aliasPrefix = "alias/"

// maxAliasLen bounds an alias the way a path segment should be bounded; a
// project alias is a short handle, not a description.
const maxAliasLen = 64

// NormalizeAlias validates a user-assigned project alias and returns its
// canonical form: trimmed and lowercased. Lowercasing mirrors NormalizeRemote,
// and for the same reason — two spellings of one alias must not become two
// buckets that a case-folding filesystem then collapses into one. Allowed are
// letters, digits, '.', '_' and '-', starting with a letter or digit and not
// ending in a dot, so the name is a single safe path component on every
// platform: no slash, no leading dot, no ".." on its own, no trailing dot
// (NTFS drops it, merging "acme." into "acme"), and none of the device names
// Windows refuses as directories — the store is a git repo a teammate may
// check out anywhere.
func NormalizeAlias(alias string) (string, error) {
	s := strings.ToLower(strings.TrimSpace(alias))
	if s == "" {
		return "", fmt.Errorf("empty alias")
	}
	if len(s) > maxAliasLen {
		return "", fmt.Errorf("alias longer than %d characters", maxAliasLen)
	}
	for i, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
		case i > 0 && (r == '.' || r == '_' || r == '-'):
		default:
			return "", fmt.Errorf("alias %q: use letters, digits, '.', '_' or '-', starting with a letter or digit", alias)
		}
	}
	if strings.HasSuffix(s, ".") {
		return "", fmt.Errorf("alias %q: must not end in a dot", alias)
	}
	if windowsReserved(s) {
		return "", fmt.Errorf("alias %q: a name Windows reserves for devices", alias)
	}
	return s, nil
}

// windowsReserved reports a name Windows refuses as a file or directory: the
// device names con, prn, aux, nul, and com/lpt followed by one digit — checked
// on the part before the first dot, since an extension doesn't lift the rule.
// (Windows also reads the superscript digits ¹²³ as digits; NormalizeAlias's
// charset already keeps those out.)
func windowsReserved(name string) bool {
	base := strings.SplitN(name, ".", 2)[0]
	switch base {
	case "con", "prn", "aux", "nul":
		return true
	}
	return len(base) == 4 && (strings.HasPrefix(base, "com") || strings.HasPrefix(base, "lpt")) &&
		base[3] >= '0' && base[3] <= '9'
}

// AliasKey returns the store key for a normalized alias (see NormalizeAlias).
// It is the alias fallback for ProjectKey: a project with no git remote has no
// host/path to be keyed by, so the user assigns a name and promote/pull key the
// project by alias/<name> instead.
func AliasKey(name string) string { return aliasPrefix + name }

// IsAliasKey reports whether key was derived from an alias rather than a
// remote, so the UI can say which kind of key it is showing.
func IsAliasKey(key string) bool { return strings.HasPrefix(key, aliasPrefix) }

// CleanAliases validates a projectAliases map as read from the config: every
// value normalized, a malformed name dropped, and a name two memory dirs both
// claim dropped from both — pull would treat one key in two memory dirs as one
// repository cloned twice, right for a remote key and wrong for two projects,
// and promote would place both under it. Consumers read only the cleaned map,
// so promote and pull agree by construction. What was dropped comes back as
// one line each, so the caller can say so rather than discard silently. The
// input is left untouched.
func CleanAliases(raw map[string]string) (clean map[string]string, dropped []string) {
	owners := map[string][]string{}
	for memDir, alias := range raw {
		name, err := NormalizeAlias(alias)
		if err != nil {
			dropped = append(dropped, memDir+": "+err.Error())
			continue
		}
		owners[name] = append(owners[name], memDir)
	}
	clean = make(map[string]string, len(owners))
	for name, dirs := range owners {
		if len(dirs) == 1 {
			clean[dirs[0]] = name
			continue
		}
		sort.Strings(dirs)
		dropped = append(dropped, name+" is claimed by "+strings.Join(dirs, " and "))
	}
	sort.Strings(dropped)
	return clean, dropped
}

// SetAlias assigns alias to memDir in a raw projectAliases map, normalized,
// enforcing one alias per project: it is refused when another memory dir
// already holds the name. A holder whose directory is gone still owns the
// store bucket its memories were shared under, so the name is freed
// deliberately — by removing that entry — never taken over. The error names
// the holder.
func SetAlias(aliases map[string]string, memDir, alias string) error {
	name, err := NormalizeAlias(alias)
	if err != nil {
		return err
	}
	for dir, other := range aliases {
		if dir == memDir {
			continue
		}
		if n, _ := NormalizeAlias(other); n == name {
			return fmt.Errorf("alias %s already keys %s — pick another; to reuse the name, remove that entry from projectAliases in the config", name, dir)
		}
	}
	aliases[memDir] = name
	return nil
}
