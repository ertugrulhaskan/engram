// Package secrets scans memory content for credentials before it is shared to a
// team git store. It is pure (no IO, no UI), so engram's promote guard can refuse
// to push a memory that looks like it carries a key.
//
// Three layers, narrowest first: a curated set of high-confidence regexes for
// known key formats, a generic rule for secret-ish variable names, and an entropy
// check for a value that looks generated under a name that gives nothing away.
// Together they catch the common cases, not everything — the guard pairs them with
// an informed override for the rest.
package secrets

import (
	"math"
	"regexp"
	"sort"
	"strconv"
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
	line int // 1-based line the segment starts on — the line a finding reports
	end  int // 1-based line it ends on; equal to line unless a wrap joined more
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
	// Normalise CRLF before segmenting. Both segmenters split on "\n", which
	// leaves a trailing \r on every line of a CRLF file — and that is enough to
	// disable the logical pass outright: neither the backslash continuation nor
	// the token-run join can see past it, so a credential split across two CRLF
	// lines is found by neither pass and reaches the store. Replacing the pair
	// keeps line numbering intact, and team.sameContent normalises the same way,
	// so a CRLF memory is already a real case elsewhere in the codebase.
	content = strings.ReplaceAll(content, "\r\n", "\n")
	type key struct {
		rule string
		line int
	}
	out := scanSegments(physicalSegments(content), scope)
	seen := make(map[key]bool, len(out))
	for _, f := range out {
		seen[key{f.Rule, f.Line}] = true
	}
	// A logical finding reports the line its segment *starts* on, which is not
	// where the secret necessarily sits — so keying the dedup on that line alone
	// let one secret be reported twice, the second time against a line holding
	// nothing. A logical hit is a duplicate when the physical pass already named
	// that rule anywhere the segment spans.
	for _, seg := range logicalSegments(content) {
		for _, f := range scanSegments([]segment{seg}, scope) {
			dup := false
			for ln := seg.line; ln <= seg.end && !dup; ln++ {
				dup = seen[key{f.Rule, ln}]
			}
			if dup {
				continue
			}
			seen[key{f.Rule, f.Line}] = true
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
		providerHit, namedHit := false, false
		for _, r := range rules {
			if r.pii && scope != ScopeSecretsAndPII {
				continue
			}
			if r.name == "generic-secret-assignment" && providerHit {
				continue // the precise provider rule already named this secret
			}
			if m := r.re.FindString(seg.text); m != "" {
				out = append(out, Finding{Rule: r.name, Line: seg.line, Match: redact(m)})
				if !r.pii {
					namedHit = true
					if r.name != "generic-secret-assignment" {
						providerHit = true
					}
				}
			}
		}
		// The entropy layer is the last resort: it only speaks when no rule named
		// a secret on this segment, so a recognized key is never reported twice.
		//
		// Note what redact leaves visible here. For a provider key the surviving
		// prefix is a format marker (AKIA, ghp_) and carries nothing secret; for an
		// unnamed value there is no marker, so those four characters are four
		// characters of the value itself. Kept deliberately: the finding is shown
		// only to the person whose file it already is, it is never logged, and
		// without them there is nothing to locate the value by. Four characters do
		// not meaningfully narrow a search for the rest.
		if !namedHit {
			if m := highEntropyRun(seg.text); m != "" {
				out = append(out, Finding{Rule: "high-entropy-string", Line: seg.line, Match: redact(m)})
			}
		}
	}
	return out
}

// The entropy layer catches what the pattern layers structurally cannot: a
// blandly-named variable holding a raw generated value, matched by neither a
// provider format nor a secret-ish identifier. Promote pushes to git, where a
// leaked credential is effectively permanent, so this is the one path where a
// false negative cannot be walked back.
//
// Thresholds are set by measurement, not taste — TestEntropyCorpus re-runs that
// measurement against any real corpus. Shannon entropy of a string is bounded by
// log2(len), so entropyMinLen and entropyMin are not independent: at 28 characters
// the ceiling is 4.81, which is what keeps the bar reachable for real keys while
// hex-shaped text (SHAs, UUIDs — around 4.0 at best) stays under it without
// special-casing.
const (
	entropyMinLen = 28
	entropyMin    = 4.4
)

// entropyCandidate finds the runs the entropy layer judges: unbroken stretches of
// the credential alphabet long enough to be a generated value.
var entropyCandidate = regexp.MustCompile(`[A-Za-z0-9+/=_-]{` + itoa(entropyMinLen) + `,}`)

// highEntropyRun returns the first run in s that looks generated rather than
// written, or "" when none does.
func highEntropyRun(s string) string {
	for _, run := range entropyCandidate.FindAllString(s, -1) {
		if looksGenerated(run) {
			return run
		}
	}
	return ""
}

// looksGenerated reports whether a run reads as machine-made.
//
// Entropy alone is not enough, and measuring a real corpus is what showed why: a
// filesystem path is one unbroken run (`/` and `-` are both base64 characters),
// it mixes case, and at 55-70 characters it scores 4.45-4.62 — above any bar a
// 28-character key could also clear, since that length caps entropy at 4.81.
// Raising the threshold would trade those false positives for missed keys, so the
// discriminator has to be structural instead: a path or a slug is separator-joined
// words, where a credential is one opaque token.
func looksGenerated(run string) bool {
	if shannon(run) < entropyMin {
		return false
	}
	if wordComposed(run) {
		return false
	}
	var lower, upper, digit bool
	for _, r := range run {
		switch {
		case r >= 'a' && r <= 'z':
			lower = true
		case r >= 'A' && r <= 'Z':
			upper = true
		case r >= '0' && r <= '9':
			digit = true
		}
	}
	// Two of the three classes: enough to exclude a lowercase slug or an
	// all-caps constant, without demanding a shape real keys may not have.
	n := 0
	for _, ok := range []bool{lower, upper, digit} {
		if ok {
			n++
		}
	}
	return n >= 2
}

// wordComposed reports whether a run is built out of separator-joined words —
// the shape of a path (`/Users/someone/workspace`), a slugified title, or a
// dotless URL fragment, none of which are credentials however well they score.
//
// A word here is a digit-free piece of word-ish length. Digit-free is cheap for
// text and awkward for a token, since digits are 16% of the base64 alphabet; the
// length bound matters because a long digit-free stretch does turn up inside real
// keys by chance, and counting one as a word would let the veto reject a genuine
// key.
//
// Lengths are byte counts, which is exact here for the same reason it is in
// tailRun: this only ever sees entropyCandidate matches, and that alphabet is
// ASCII.
//
// The counts are measured, not chosen. Against 200,000 synthetic values per
// format, requiring three words rejected 1 real key in 360 — far too many for a
// guard whose misses are permanent — while five rejected between 1 in 200,000 and
// 1 in 12,000, and still silenced every path and slug tested. Requiring fewer
// words but no digits anywhere was tried and is strictly worse: it rejects real
// keys 90x more often and vetoes nothing extra.
func wordComposed(run string) bool {
	pieces := strings.FieldsFunc(run, isSeparator)
	if len(pieces) < wordPieces {
		return false
	}
	words := 0
	for _, p := range pieces {
		if len(p) >= wordMinLen && len(p) <= wordMaxLen && !strings.ContainsAny(p, "0123456789") {
			words++
		}
	}
	return words >= wordPieces
}

// wordPieces is how many word-shaped pieces make a run text rather than a token;
// wordMinLen and wordMaxLen bound what counts as a word at all — wide enough for
// "src" and "instructions", narrow enough that neither a two-character fragment
// nor a long lucky stretch of a key counts toward the veto. Five is comfortably
// under what real text carries: the paths this was measured against ran to six
// and eight words, and a run has to reach 28 characters before it is judged at
// all, which is already several words long.
const (
	wordPieces = 5
	wordMinLen = 3
	wordMaxLen = 14
)

// isSeparator reports whether r joins pieces rather than belonging to a value.
// All five are in the credential alphabet — that is exactly why a path survives
// entropyCandidate as one run — so the veto reads the same characters the
// candidate pattern does, just structurally.
func isSeparator(r rune) bool {
	return r == '-' || r == '_' || r == '/' || r == '+' || r == '='
}

// shannon is the Shannon entropy of s in bits per character.
func shannon(s string) float64 {
	if s == "" {
		return 0
	}
	var counts [256]int
	for i := 0; i < len(s); i++ {
		counts[s[i]]++
	}
	n := float64(len(s))
	e := 0.0
	for _, c := range counts {
		if c == 0 {
			continue
		}
		p := float64(c) / n
		e -= p * math.Log2(p)
	}
	return e
}

// itoa keeps the candidate pattern in sync with entropyMinLen, so the two cannot
// drift apart when the threshold is retuned.
func itoa(n int) string { return strconv.Itoa(n) }

// physicalSegments is one segment per line, exactly as the scanner always read
// content.
func physicalSegments(content string) []segment {
	lines := strings.Split(content, "\n")
	segs := make([]segment, len(lines))
	for i, ln := range lines {
		segs[i] = segment{text: ln, line: i + 1, end: i + 1}
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
	cur, start, end := lines[0], 1, 1
	for i := 1; i < len(lines); i++ {
		next := strings.TrimLeft(lines[i], " \t")
		if strings.HasSuffix(cur, `\`) {
			cur = strings.TrimSuffix(cur, `\`) + next
			end = i + 1
			continue
		}
		if tokenLike(cur[len(cur)-tailRun(cur):]) && tokenLike(next[:headRun(next)]) {
			cur += next
			end = i + 1
			continue
		}
		segs = append(segs, segment{text: cur, line: start, end: end})
		cur, start, end = lines[i], i+1, i+1
	}
	return append(segs, segment{text: cur, line: start, end: end})
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
