// Package secrets scans memory content for credentials before it is shared to a
// team git store. It is pure (no IO, no UI): a curated set of high-confidence
// regexes over the text, so engram's promote guard can refuse to push a memory
// that looks like it carries a key. A curated set catches the common formats, not
// everything — the guard pairs it with an informed override for the rest.
package secrets

import (
	"regexp"
	"strings"
)

// Scope selects which rule classes Scan applies.
type Scope int

const (
	ScopeSecrets       Scope = iota // credentials / API keys / tokens only (default)
	ScopeSecretsAndPII              // also emails and card-like numbers (noisier)
)

// Finding is one match: which rule fired, the 1-based line, and a redacted
// preview (never the full secret).
type Finding struct {
	Rule  string
	Line  int
	Match string
}

type rule struct {
	name string
	re   *regexp.Regexp
	pii  bool
}

// rules are ordered most-specific first so a value is labelled by its precise
// provider rule before the generic assignment rule can claim it.
var rules = []rule{
	{"aws-access-key-id", regexp.MustCompile(`AKIA[0-9A-Z]{16}`), false},
	{"github-token", regexp.MustCompile(`gh[oprsu]_[0-9A-Za-z]{36}`), false},
	{"github-fine-grained-pat", regexp.MustCompile(`github_pat_[0-9A-Za-z_]{22,}`), false},
	{"anthropic-api-key", regexp.MustCompile(`sk-ant-[0-9A-Za-z_-]{20,}`), false},
	{"openai-api-key", regexp.MustCompile(`sk-(?:proj|svcacct|admin)?-?[A-Za-z0-9]{20,}`), false},
	{"stripe-secret-key", regexp.MustCompile(`[sr]k_(?:live|test)_[A-Za-z0-9]{16,}`), false},
	{"google-api-key", regexp.MustCompile(`AIza[0-9A-Za-z_-]{35}`), false},
	{"slack-token", regexp.MustCompile(`xox[baprs]-[0-9A-Za-z-]{10,}`), false},
	{"slack-webhook", regexp.MustCompile(`https://hooks\.slack\.com/services/[A-Za-z0-9/]+`), false},
	{"url-credentials", regexp.MustCompile(`[a-z][a-z0-9+.-]*://[^\s:/@]*:[^\s/@]+@`), false},
	{"private-key-block", regexp.MustCompile(`-----BEGIN (?:RSA |EC |OPENSSH |DSA |PGP |ENCRYPTED )?PRIVATE KEY-----`), false},
	{"jwt", regexp.MustCompile(`eyJ[A-Za-z0-9_-]{10,}\.eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}`), false},
	{"generic-secret-assignment", regexp.MustCompile(`(?i)[\w.-]*(?:secret|token|passw(?:or)?d|api[_-]?key|access[_-]?key|private[_-]?key|client[_-]?secret)[\w.-]*["']?\s*[:=]\s*["']?[A-Za-z0-9/+=_-]{12,}`), false},
	{"email-address", regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`), true},
	{"card-number", regexp.MustCompile(`\b(?:\d[ -]?){13,16}\b`), true},
}

// Scan returns the secrets found in content (empty when clean). Each line is
// checked against every in-scope rule, but the generic assignment rule is
// suppressed on a line a precise provider rule already matched, so one secret is
// not reported twice.
func Scan(content string, scope Scope) []Finding {
	var out []Finding
	for i, line := range strings.Split(content, "\n") {
		providerHit := false
		for _, r := range rules {
			if r.pii && scope != ScopeSecretsAndPII {
				continue
			}
			if r.name == "generic-secret-assignment" && providerHit {
				continue // the precise provider rule already named this secret
			}
			if m := r.re.FindString(line); m != "" {
				out = append(out, Finding{Rule: r.name, Line: i + 1, Match: redact(m)})
				if !r.pii && r.name != "generic-secret-assignment" {
					providerHit = true
				}
			}
		}
	}
	return out
}

// redact keeps only a short recognizable prefix (enough to tell an AWS key from a
// JWT) and masks the rest, capped so a long value can't be reconstructed and no
// trailing characters of the secret leak.
func redact(s string) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= 4 {
		return strings.Repeat("•", len(r))
	}
	dots := len(r) - 4
	if dots > 8 {
		dots = 8
	}
	return string(r[:4]) + strings.Repeat("•", dots)
}
