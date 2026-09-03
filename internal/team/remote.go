package team

import (
	"errors"
	"fmt"
	"strings"
)

// ErrReservedHost reports a remote engram refuses to key because its host is
// "alias", which would collide with the projects/alias/<name>/ namespace
// AliasKey owns. It is its own error so the UI can name the real cause — git
// answered correctly here — instead of reporting it as "git could not say".
var ErrReservedHost = errors.New("remote host is reserved")

// NormalizeRemote canonicalizes a git remote URL into a stable "host/path" key
// used to match the same project across machines regardless of clone path or URL
// form. It accepts scheme (https/ssh/git), scp-like, and authenticated variants,
// stripping the scheme, userinfo, port, ".git" suffix, and surrounding slashes,
// and lowercasing host and path. For example all of
//
//	https://github.com/Acme/App.git
//	git@github.com:acme/app.git
//	ssh://git@github.com:2222/acme/app
//
// normalize to "github.com/acme/app".
func NormalizeRemote(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", fmt.Errorf("empty remote URL")
	}

	var host, path string
	if i := strings.Index(s, "://"); i >= 0 {
		// scheme://[user@]host[:port]/path
		rest := s[i+3:]
		host, path = rest, ""
		if j := strings.Index(rest, "/"); j >= 0 {
			host, path = rest[:j], rest[j+1:]
		}
		host = stripUserinfo(host)
		if j := strings.Index(host, ":"); j >= 0 { // strip :port
			host = host[:j]
		}
	} else if strings.Contains(s, ":") {
		// scp-like [user@]host:path — strip userinfo first so a ':' inside it
		// (user:pass@host) isn't mistaken for the host/path separator.
		hostPath := s
		if at := strings.LastIndex(s, "@"); at >= 0 {
			hostPath = s[at+1:]
		}
		j := strings.Index(hostPath, ":")
		if j < 0 {
			return "", fmt.Errorf("unrecognized remote URL: %q", raw)
		}
		host, path = hostPath[:j], hostPath[j+1:]
	} else {
		return "", fmt.Errorf("unrecognized remote URL: %q", raw)
	}

	path = strings.Trim(path, "/")
	path = strings.TrimSuffix(path, ".git")
	path = strings.Trim(path, "/")

	// A trailing dot is the root label of an FQDN and means the same host; a
	// trailing space is never meaningful. Both are trimmed for the reason
	// NormalizeAlias refuses them in a name: NTFS strips them, so "alias." and
	// "alias " would be distinct directories here and one directory once a
	// teammate checks the store out on Windows — collapsing into the very
	// namespace the reserved-host check below exists to keep separate.
	host = strings.TrimRight(strings.ToLower(host), ". ")
	path = strings.ToLower(path)

	if host == "" {
		return "", fmt.Errorf("remote URL has no host: %q", raw)
	}
	if path == "" {
		return "", fmt.Errorf("remote URL has no path: %q", raw)
	}
	if host == "alias" {
		// projects/alias/<name>/ is engram's namespace for alias-derived keys
		// (AliasKey), and a remote whose host is literally "alias" — an
		// ssh_config Host of that name — normalizes straight into it. For a
		// single-segment path the collision is exact, not adjacent:
		// "alias:acme" gives "alias/acme", which *is* AliasKey("acme"). Two
		// unrelated projects would then share one store bucket, and applyPull's
		// byKey would list both memory dirs under that key and cross-place each
		// project's memories into the other.
		//
		// So this is refused, and refused with its own error rather than a
		// generic one: ClassifyRemote turns it into RemoteReserved so the
		// dialogs can say what actually happened and how to fix it. An earlier
		// pass let it through on the argument that IsAliasKey only decides a
		// caption — true for multi-segment paths, where the remote lands in a
		// subdirectory the alias bucket never reads, and false for exactly the
		// case above.
		return "", fmt.Errorf("%w: %q is engram's alias-key namespace — rename the ssh alias", ErrReservedHost, host)
	}
	return host + "/" + path, nil
}

// stripUserinfo drops a leading "user@" (or "user:pass@") from a host segment.
func stripUserinfo(host string) string {
	if i := strings.LastIndex(host, "@"); i >= 0 {
		return host[i+1:]
	}
	return host
}
