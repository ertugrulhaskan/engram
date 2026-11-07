package secrets

import (
	"regexp"
	"strings"
	"unicode"
)

// genericRule names both assignment rules: the base64-shaped one in the rule
// table and mixedSecretAssignment. One name, because to the person reading the
// finding they are the same thing — a secret-named key holding a value.
const genericRule = "generic-secret-assignment"

// secretKeywords are the substrings that make a key name a secret.
var secretKeywords = []string{
	"password", "passwd", "secret", "token", "credential",
	"apikey", "api_key", "api-key", "access_key", "private_key",
}

// referenceValue reports whether a matched value is a placeholder or a
// reference rather than a secret: a run of one or two distinct characters
// (xxxxxxxx, ********), or an IDENTIFIER that names the secret without holding
// it — CONSTANT_CASE (DB_PASSWORD_FROM_VAULT) or a dotted path (config.apiKey)
// that contains a secret word and no digit.
//
// The identifier shape is what keeps this narrow. "Contains a secret word, no
// digit" alone also matched weak real passwords like mysecretpassword or
// my-token-value-here, which the guard has always reported; a miss here is
// permanent (the value lands in git), so anything that isn't plainly an
// identifier is still treated as a secret.
func referenceValue(v string) bool {
	if distinctRunes(v) <= 2 {
		return true
	}
	if strings.ContainsAny(v, "0123456789") || !identifierShaped(v) {
		return false
	}
	lv := strings.ToLower(v)
	for _, k := range secretKeywords {
		if strings.Contains(lv, k) {
			return true
		}
	}
	return false
}

// identifierShaped reports whether v reads as a code identifier: letters only
// (plus "_" and "."), and either CONSTANT_CASE with an underscore or a dotted
// path. camelCase is deliberately not enough: MySecretPassword is as likely a
// real password as a variable name, and the guard has always reported it.
func identifierShaped(v string) bool {
	var upper, lower bool
	for _, r := range v {
		switch {
		case unicode.IsUpper(r):
			upper = true
		case unicode.IsLower(r):
			lower = true
		case r == '_' || r == '.':
		default:
			return false
		}
	}
	switch {
	case upper && !lower && strings.Contains(v, "_"):
		return true // CONSTANT_CASE
	case strings.Contains(v, "."):
		return true // dotted path
	default:
		return false
	}
}

// assignment finds key = value / key: value pairs, where the value runs to the
// next whitespace or quote and is at least 6 characters (Abc123! is a real
// password; plausibleSecretValue does the filtering, not the length). The rule table's assignment rule only accepts a
// base64-shaped value of 12 or more characters, so a password with a symbol in
// its first twelve characters, or under twelve long, matched nothing.
//
// Markdown emphasis around the key or the colon is allowed — "**Password:** x"
// and "**Password**: x" are how memories often write a credential.
var assignment = regexp.MustCompile(`([A-Za-z_][\w.-]*)[*_]{0,3}["']?\s*[:=][*_]{0,3}\s*["']?([^\s"'` + "`" + `*]{6,})`)

// mixedSecretAssignment returns the first assignment whose key names a secret
// and whose value reads as one, or "".
func mixedSecretAssignment(s string) string {
	for _, m := range assignment.FindAllStringSubmatch(s, -1) {
		if keyNamesSecret(m[1]) && plausibleSecretValue(m[2]) {
			return m[0]
		}
	}
	return ""
}

// keyNamesSecret reports whether an identifier names a secret: it contains a
// secret keyword, or one of its segments is exactly "pass" or "pwd" — so DB_PASS
// and dbPwd count, while bypass, passport and compass do not.
func keyNamesSecret(key string) bool {
	lk := strings.ToLower(key)
	for _, k := range secretKeywords {
		if strings.Contains(lk, k) {
			return true
		}
	}
	for _, seg := range identSegments(key) {
		if seg == "pass" || seg == "pwd" {
			return true
		}
	}
	return false
}

// identSegments splits an identifier on _ . - and on lower→upper camelCase
// boundaries, lowercased: "DB_PASS" → [db pass], "dbPwd" → [db pwd].
func identSegments(key string) []string {
	var segs []string
	var cur []rune
	flush := func() {
		if len(cur) > 0 {
			segs = append(segs, strings.ToLower(string(cur)))
			cur = cur[:0]
		}
	}
	rs := []rune(key)
	for i, r := range rs {
		switch {
		case r == '_' || r == '.' || r == '-':
			flush()
		case unicode.IsUpper(r) && i > 0 && unicode.IsLower(rs[i-1]):
			flush()
			cur = append(cur, r)
		default:
			cur = append(cur, r)
		}
	}
	flush()
	return segs
}

// plausibleSecretValue reports whether an assigned value reads as a secret
// rather than code, a reference or a placeholder. It needs a letter plus a digit
// or a symbol — what passwords are made of, and what an identifier reference
// (newPassword) or a plain word (required) lacks.
//
// Every exclusion matches the WHOLE shape of a reference, never a substring or
// a single character: a miss here lands a credential in git for good, and the
// first version, which refused any value containing "your", "example", a
// bracket, or starting with "%", waved through real passwords like
// Yourbaby123!, P@ss(word)1x and %Secret123!x.
func plausibleSecretValue(v string) bool {
	// Trailing ",", ";", ")" and ":" close a line of code, not a password:
	// `'x-api-key': config.apiKey,`, `fn(tokens: TokenSet, …)` and
	// `refresh(tokens: TokenSet): Promise<…>` all matched on them, measured
	// against a real tree of memories. Whatever remains is still judged.
	if codeReference(v) { // before trimming: a call keeps its ")" (getSecret())
		return false
	}
	v = strings.TrimRight(v, ",;):")
	if v == "" || referenceValue(v) || codeReference(v) || placeholder(v) {
		return false
	}
	if isoDate.MatchString(v) {
		return false
	}
	var letter, digit, symbol bool
	for _, r := range v {
		switch {
		case unicode.IsLetter(r):
			letter = true
		case unicode.IsDigit(r):
			digit = true
		case !strings.ContainsRune("_-./", r):
			symbol = true
		}
	}
	return letter && (digit || symbol)
}

// codeShapes are values that point at a secret in code or config: an env-var
// reference ($DB_PASS, ${DB_PASS}, %DB_PASS%), a templating or placeholder
// wrapper (<your-token>, {{ .Values.x }}), a call or method call
// (z.string().min(8), getSecret()), an index or generic (Optional[str],
// os.environ["KEY"]), or a URL or filesystem path.
var codeShapes = []*regexp.Regexp{
	regexp.MustCompile(`^\$(?:\{|[A-Z_][A-Z0-9_]*$)`),
	regexp.MustCompile(`^%[A-Za-z_][A-Za-z0-9_]*%$`),
	regexp.MustCompile(`^<[^<>]*>$`),
	regexp.MustCompile(`^\{\{.*\}\}$|\$\{[A-Za-z_][\w.]*\}`),
	regexp.MustCompile(`^[A-Za-z_]\w*(?:\.[A-Za-z_]\w*|\([^()]*\))*\([^()]*\)(?:\.[A-Za-z_]\w*|\([^()]*\))*$`),
	regexp.MustCompile(`^[A-Za-z_][\w.]*\[.*\]$`),
	regexp.MustCompile(`^[a-z][a-z0-9+.-]*://`),
	regexp.MustCompile(`^(?:~|\.\.?)/|^/[^/]+/`),
	regexp.MustCompile(`^(?:process\.env|os\.environ|os\.getenv|getenv)\b`),
}

func codeReference(v string) bool {
	for _, re := range codeShapes {
		if re.MatchString(v) {
			return true
		}
	}
	return false
}

// placeholder reports a value that is filler rather than a credential: masked
// runs (xxxx, ****, ...) or a separator-joined phrase built from placeholder
// words (your-token-here, example_key) with no digit. Words count only as whole
// tokens, so Yourbaby123! or MyExample#Pass9 are still reported.
func placeholder(v string) bool {
	lv := strings.ToLower(v)
	if strings.Contains(lv, "xxxx") || strings.Contains(v, "****") || strings.Contains(v, "...") {
		return true
	}
	if strings.ContainsAny(v, "0123456789") {
		return false
	}
	for _, tok := range strings.FieldsFunc(lv, func(r rune) bool { return strings.ContainsRune("-_ ()[]<>", r) }) {
		switch tok {
		case "your", "example", "placeholder", "redacted", "dummy", "sample", "changeme":
			return true
		}
	}
	return false
}

var isoDate = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}`)

func distinctRunes(s string) int {
	seen := map[rune]bool{}
	for _, r := range s {
		seen[r] = true
	}
	return len(seen)
}
