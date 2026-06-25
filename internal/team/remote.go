package team

import (
	"fmt"
	"strings"
)

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

	host = strings.ToLower(host)
	path = strings.ToLower(path)

	if host == "" {
		return "", fmt.Errorf("remote URL has no host: %q", raw)
	}
	if path == "" {
		return "", fmt.Errorf("remote URL has no path: %q", raw)
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
