// Package secrets scans memory content for credentials before it is shared to a
// team git store. It is pure (no IO, no UI): a curated set of high-confidence
// regexes over the text, so engram's promote guard can refuse to push a memory
// that looks like it carries a key. A curated set catches the common formats, not
// everything — the guard pairs it with an informed override for the rest.
package secrets

import (
	"regexp"
	"sort"
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
	{"private-key-block", regexp.MustCompile(`-----BEGIN (?:RSA |EC |OPENSSH |DSA |PGP |ENCRYPTED )?PRIVATE KEY(?: BLOCK)?-----`), false},
	{"jwt", regexp.MustCompile(`eyJ[A-Za-z0-9_-]{10,}\.eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}`), false},
	{"generic-secret-assignment", regexp.MustCompile(`(?i)[\w.-]*(?:secret|token|passw(?:or)?d|api[_-]?key|access[_-]?key|private[_-]?key|client[_-]?secret)[\w.-]*["']?\s*[:=]\s*["']?[A-Za-z0-9/+=_-]{12,}`), false},
	{"email-address", regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`), true},
	{"card-number", regexp.MustCompile(`\b(?:\d[ -]?){13,16}\b`), true},
}

// segment is one unit of text handed to the rules, tagged with the 1-based source
// line it starts on — so a match spanning a wrap still reports where it began.
type segment struct {
	text string
	line int
}

// Scan returns the secrets found in content (empty when clean).
//
// Two passes over the same rules. The first sees physical lines, which keeps the
// precision the rules were tuned for. The second sees *logical* lines, with the
// breaks that merely wrap a token removed, so a credential split across lines is
// still caught — a structural gap no extra regex could close.
//
// A finding the physical pass already reported is not repeated: when nothing in
// the content is wrapped the two passes produce identical results and the second
// contributes nothing at all, which is what keeps this from changing established
// behavior.
func Scan(content string, scope Scope) []Finding {
	type key struct {
		rule string
		line int
	}
	out := scanSegments(physicalSegments(content), scope)
	seen := make(map[key]bool, len(out))
	for _, f := range out {
		seen[key{f.Rule, f.Line}] = true
	}
	for _, f := range scanSegments(logicalSegments(content), scope) {
		if k := (key{f.Rule, f.Line}); !seen[k] {
			seen[k] = true
			out = append(out, f)
		}
	}
	// Keep the report in source order; the wrapped findings are discovered last
	// but belong beside their neighbours. Stable, so same-line findings keep the
	// most-specific-rule-first order the rule table gives them.
	sort.SliceStable(out, func(i, j int) bool { return out[i].Line < out[j].Line })
	return out
}

// scanSegments applies every in-scope rule to each segment. The generic
// assignment rule is suppressed on a segment a precise provider rule already
// matched, so one secret is not reported twice.
func scanSegments(segs []segment, scope Scope) []Finding {
	var out []Finding
	for _, seg := range segs {
		providerHit := false
		for _, r := range rules {
			if r.pii && scope != ScopeSecretsAndPII {
				continue
			}
			if r.name == "generic-secret-assignment" && providerHit {
				continue // the precise provider rule already named this secret
			}
			if m := r.re.FindString(seg.text); m != "" {
				out = append(out, Finding{Rule: r.name, Line: seg.line, Match: redact(m)})
				if !r.pii && r.name != "generic-secret-assignment" {
					providerHit = true
				}
			}
		}
	}
	return out
}

// physicalSegments is one segment per line, exactly as the scanner always read
// content.
func physicalSegments(content string) []segment {
	lines := strings.Split(content, "\n")
	segs := make([]segment, len(lines))
	for i, ln := range lines {
		segs[i] = segment{text: ln, line: i + 1}
	}
	return segs
}

// wrapRun is the shortest credential-alphabet run that can sit either side of a
// wrapped value, and longWrapRun the length at which such a run is convincing on
// its own. Between the two, a digit is required — which is what separates a token
// fragment from an English word, since ordinary prose fuses otherwise ("continues"
// and "understanding" are both long enough to look like a wrap without this).
const (
	wrapRun     = 8
	longWrapRun = 16
)

// tokenLike reports whether a run of credential characters reads as part of a
// split secret rather than a word ending a sentence. Credentials carry digits or
// run long; English words seldom do either.
func tokenLike(run string) bool {
	if len(run) < wrapRun {
		return false
	}
	if len(run) >= longWrapRun {
		return true
	}
	return strings.ContainsAny(run, "0123456789")
}

// logicalSegments rebuilds content with the line breaks that merely wrap a single
// value removed, so a rule can see the whole value. Every segment keeps the line
// its first character came from, which is the line a spanning match reports.
//
// A break is a wrap when the previous line ends with an explicit backslash
// continuation, or when a run of credential characters ends one line and another
// begins the next — after leading indentation is dropped, so a YAML block scalar
// joins the way a soft-wrapped paste does.
func logicalSegments(content string) []segment {
	lines := strings.Split(content, "\n")
	var segs []segment
	cur, start := lines[0], 1
	for i := 1; i < len(lines); i++ {
		next := strings.TrimLeft(lines[i], " \t")
		if strings.HasSuffix(cur, `\`) {
			cur = strings.TrimSuffix(cur, `\`) + next
			continue
		}
		if tokenLike(cur[len(cur)-tailRun(cur):]) && tokenLike(next[:headRun(next)]) {
			cur += next
			continue
		}
		segs = append(segs, segment{text: cur, line: start})
		cur, start = lines[i], i+1
	}
	return append(segs, segment{text: cur, line: start})
}

// isWrapChar reports whether r can appear inside a wrapped credential — the
// base64/token alphabet, which deliberately excludes whitespace and punctuation
// that would end a value.
func isWrapChar(r rune) bool {
	return r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' ||
		r == '+' || r == '/' || r == '=' || r == '_' || r == '-'
}

// tailRun counts the credential characters ending s.
func tailRun(s string) int {
	r := []rune(s)
	n := 0
	for n < len(r) && isWrapChar(r[len(r)-1-n]) {
		n++
	}
	return n
}

// headRun counts the credential characters starting s.
func headRun(s string) int {
	n := 0
	for _, r := range s {
		if !isWrapChar(r) {
			break
		}
		n++
	}
	return n
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
