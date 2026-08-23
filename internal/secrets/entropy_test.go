package secrets

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// TestEntropyCatchesBlandlyNamedSecrets pins the gap the entropy layer exists to
// close: a generated value under a name no rule recognizes. The named rules see
// nothing here — neither the value's format nor the identifier holding it says
// "secret" — so a hit can only come from the value's own shape.
func TestEntropyCatchesBlandlyNamedSecrets(t *testing.T) {
	cases := []struct{ name, content string }{
		{"bland assignment", "deploy: Xk9mPq2vRt7wYz4aBc8dEf1gHj5kLn3p"},
		{"bare value in prose", "Paste this when it asks: 7fQz2XmK9pL4vNb8RtY6wCj3HdG5sA1eUo"},
		{"yaml value", "config:\n  value: Bq7XzM2nP9kR4tV6yH8jL3wC5dF1gS0aZeUiOp"},
		{"env line", "DEPLOY=Zm4KpX9qL2vR7bN5tY8wHj3cD6fG1sA0eU"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Scan(c.content, ScopeSecrets)
			if len(got) != 1 || got[0].Rule != "high-entropy-string" {
				t.Fatalf("want one high-entropy-string finding, got %+v", got)
			}
			if strings.Contains(c.content, got[0].Match) {
				t.Errorf("finding leaked the raw value: %q", got[0].Match)
			}
			if !strings.Contains(got[0].Match, "•") {
				t.Errorf("finding is not redacted: %q", got[0].Match)
			}
		})
	}
}

// TestEntropyStartsAtItsLengthFloor pins entropyMinLen against entropyMin, which
// only make sense together: entropy is capped at log2(len), so a shorter floor
// means judging runs whose ceiling is barely over the bar, where scoring high says
// more about being short than about being generated. One character under the floor
// is deliberately silent, however well it would have scored.
func TestEntropyStartsAtItsLengthFloor(t *testing.T) {
	const long = "Xk9mPq2vRt7wYz4aBc8dEf1gHj5n" // 28 characters, all distinct
	short := long[:len(long)-1]

	if s := shannon(short); s < entropyMin {
		t.Fatalf("the short case scores %.3f, under the bar for other reasons — it no longer tests the floor", s)
	}
	if got := Scan("deploy: "+short, ScopeSecrets); len(got) != 0 {
		t.Errorf("a run under entropyMinLen was reported: %+v", got)
	}
	if got := Scan("deploy: "+long, ScopeSecrets); len(got) != 1 {
		t.Errorf("a run at entropyMinLen was not reported: %+v", got)
	}
}

// TestEntropyDefersToNamedRules keeps the layer last in line. A recognized key
// must be reported by the rule that can name it and not a second time as an
// anonymous blob, or the promote guard would show every real secret twice.
func TestEntropyDefersToNamedRules(t *testing.T) {
	cases := []struct{ name, content, wantRule string }{
		{"provider format", "AKIA" + strings.Repeat("Q7RZ", 4), "aws-access-key-id"},
		{"secret-ish name", "api_key = Xk9mPq2vRt7wYz4aBc8dEf1gHj5kLn3p", "generic-secret-assignment"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Scan(c.content, ScopeSecrets)
			if len(got) != 1 {
				t.Fatalf("want exactly one finding, got %+v", got)
			}
			if got[0].Rule != c.wantRule {
				t.Errorf("rule = %q, want %q", got[0].Rule, c.wantRule)
			}
		})
	}
}

// TestEntropyIgnoresWordShapedText covers what measuring a real corpus turned up.
// Every false positive there was a filesystem path: `/` and `-` are both base64
// characters, so a path is one unbroken run, and a long one scores above the bar a
// 28-character key has to be able to clear.
//
// Each case is checked to clear entropyMin first. Without that the test would pass
// for the wrong reason — an ordinary short path scores around 4.0 and is already
// silent on the threshold alone, so it proves nothing about the veto. These are
// quiet structurally or not at all.
func TestEntropyIgnoresWordShapedText(t *testing.T) {
	quiet := []struct{ name, content string }{
		{"claude project dir key", "under /-Users-jbaker-Documents-workspace-quartz-flux-bindery/memory/MEMORY"},
		{"nested project dir key", "see /-Users-mzhang-Documents-workspace-vexing-jumble-kit/memory/MEMORY"},
		{"absolute path", "at /Users/pquinn/Documents/workspace/zephyr-badge-mkII/vendor/Manifest"},
		{"cache path", "under /Users/jbaker/Library/Caches/quartz-flux/BuildPhaseZ/Manifest"},
		{"mixed separators", "key /-Users-dkovacs-Documents-workspace-glyph-vortex-qube/memory/MEMORY"},
	}
	for _, c := range quiet {
		t.Run(c.name, func(t *testing.T) {
			scored := false
			for _, run := range entropyCandidate.FindAllString(c.content, -1) {
				if shannon(run) >= entropyMin {
					scored = true
				}
			}
			if !scored {
				t.Fatalf("no run here reaches entropyMin, so this case no longer exercises the veto")
			}
			if got := Scan(c.content, ScopeSecrets); len(got) != 0 {
				t.Errorf("false positive: %+v", got)
			}
		})
	}
}

// TestEntropyIgnoresHexIdentifiers is the other half, and stays quiet for the
// other reason: hex draws on 16 symbols, so a SHA or a UUID tops out near 4.0 and
// never reaches the bar. It costs real recall — a hex-encoded secret is missed —
// which is the deliberate trade, since identifiers of exactly this shape are
// common in memories and a credential rarely is.
func TestEntropyIgnoresHexIdentifiers(t *testing.T) {
	quiet := []struct{ name, content string }{
		{"uuid", "id 550e8400-e29b-41d4-a716-446655440000"},
		{"sha1", "commit 356a192b7913b04c54574d18c28d46e6395428ab"},
		{"sha256", "digest " + strings.Repeat("9f86d081", 8)},
	}
	for _, c := range quiet {
		t.Run(c.name, func(t *testing.T) {
			if got := Scan(c.content, ScopeSecrets); len(got) != 0 {
				t.Errorf("false positive: %+v", got)
			}
		})
	}
}

// TestWordComposed pins the veto's boundaries directly. The corpus and rate tests
// judge it in aggregate, which is what makes them realistic and also what makes
// them blunt: each bound can be widened a little without moving either number
// enough to fail. These cases move one thing at a time.
func TestWordComposed(t *testing.T) {
	cases := []struct {
		name string
		run  string
		want bool
	}{
		{"five words", "alpha-bravo-charlie-delta-echo", true},
		{"four words is not enough", "alpha-bravo-charlie-delta", false},
		{"slashes count as separators", "alpha/bravo/charlie/delta/echo", true},
		{"mixed separators", "alpha-bravo/charlie_delta+echo", true},

		{"a three-character piece is a word", "alpha-bravo-charlie-delta-fox", true},
		{"a two-character piece is not", "alpha-bravo-charlie-delta-ox", false},

		{"a fourteen-character piece is a word", "alpha-bravo-charlie-delta-abcdefghijklmn", true},
		{"a fifteen-character piece is not", "alpha-bravo-charlie-delta-abcdefghijklmno", false},

		{"a piece with a digit is not a word", "alpha-bravo-charlie-delta-ech0", false},
		{"one run with no separators", "alphabravocharliedeltaechofox", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := wordComposed(c.run); got != c.want {
				t.Errorf("wordComposed(%q) = %v, want %v", c.run, got, c.want)
			}
		})
	}
}

// TestEntropyNeedsTwoCharacterClasses pins the last of the three conditions. The
// entropy bar and the word veto both pass on a single-class run — a long stretch of
// only letters scores 4.66 and splits into nothing — so this check is what stops an
// all-lowercase or all-caps identifier from reading as generated.
//
// Each case is built to isolate that: over the bar, not word-composed, so the class
// count is the only thing left deciding.
func TestEntropyNeedsTwoCharacterClasses(t *testing.T) {
	cases := []struct {
		name string
		run  string
		want bool
	}{
		{"lowercase only", "qwertyuiopasdfghjklzxcvbnmqw", false},
		{"uppercase only", "QWERTYUIOPASDFGHJKLZXCVBNMQW", false},
		{"lower and digit", "a1b2c3d4e5f6g7h8j9k0mnpqrstv", true},
		{"upper and digit", "A1B2C3D4E5F6G7H8J9K0MNPQRSTV", true},
		{"lower and upper", "aQbWcEdRfTgYhUjIkOlPmZnXvCzB", true},

		// Both ends of the digit range, so neither bound can be trimmed silently.
		{"the only digit is 0", "0abcdefghijklmnopqrstuvwxyza", true},
		{"the only digit is 9", "9abcdefghijklmnopqrstuvwxyza", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if s := shannon(c.run); s < entropyMin {
				t.Fatalf("scores %.3f, under the bar for another reason — this no longer isolates the class check", s)
			}
			if wordComposed(c.run) {
				t.Fatalf("is word-composed, so the veto decides it — this no longer isolates the class check")
			}
			if got := looksGenerated(c.run); got != c.want {
				t.Errorf("looksGenerated(%q) = %v, want %v", c.run, got, c.want)
			}
		})
	}
}

// TestEntropyDegenerateRuns covers the inputs that reach the layer only when
// something upstream is unusual — a rule of separators, a padding run, a repeated
// character. Each is long enough to be judged and carries no information at all,
// so the entropy bar alone should hold, and nothing should panic on the way.
func TestEntropyDegenerateRuns(t *testing.T) {
	for _, s := range []string{
		"", "\n", "\n\n\n",
		strings.Repeat("=", 40),
		strings.Repeat("-", 40),
		strings.Repeat("a", 40),
		strings.Repeat("-a", 20),
		strings.Repeat("ab", 20),
	} {
		if got := Scan(s, ScopeSecrets); len(got) != 0 {
			t.Errorf("Scan(%q) reported %+v", s, got)
		}
	}
}

// TestEntropyIsMultibyteSafe guards the byte-indexing in shannon and the byte
// lengths in wordComposed. Both are exact only because entropyCandidate matches
// ASCII, so a multibyte character next to a real value must not shift the match.
func TestEntropyIsMultibyteSafe(t *testing.T) {
	const key = "Xk9mPq2vRt7wYz4aBc8dEf1gHj5n"
	for _, prefix := range []string{"🔑", "café ", "日本語 ", ""} {
		got := Scan(prefix+key, ScopeSecrets)
		if len(got) != 1 || got[0].Rule != "high-entropy-string" {
			t.Errorf("Scan(%q+key) = %+v, want one high-entropy-string", prefix, got)
			continue
		}
		if got[0].Line != 1 {
			t.Errorf("Scan(%q+key) reported line %d, want 1", prefix, got[0].Line)
		}
	}
}

// --- calibration ---
//
// The alphabets and lengths below are the real shapes of generated credentials,
// so a threshold change can be judged against what it would start missing rather
// than against taste.
const (
	alphaB64    = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	alphaB64URL = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"
	alphaB62    = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
)

// synthKey draws a value of n characters from alpha. Seeded, so a failure is the
// same failure on the next run.
func synthKey(seed int64, n int, alpha string) string {
	r := rand.New(rand.NewSource(seed))
	b := make([]byte, n)
	for i := range b {
		b[i] = alpha[r.Intn(len(alpha))]
	}
	return string(b)
}

// TestWordVetoRarelyCostsARealKey is the other half of the corpus measurement.
// The veto that silenced every path buys its precision with some recall, and this
// is what pins the price.
//
// It deliberately does not assert zero. A generated value can, by luck, split into
// word-shaped pieces, so the honest claim is a rate rather than an absolute — and
// a zero assertion would only be true of whichever sample happened to be drawn.
// The bound is set an order of magnitude above the measured rate, so it fails on a
// change of behavior rather than on noise.
func TestWordVetoRarelyCostsARealKey(t *testing.T) {
	shapes := []struct {
		name  string
		n     int
		alpha string
	}{
		{"base64-32", 32, alphaB64},
		{"base64url-43", 43, alphaB64URL},
		{"base64url-64", 64, alphaB64URL},
		{"base62-32", 32, alphaB62},
		{"base62-50", 50, alphaB62},
	}
	// Measured over 200k samples per shape: 1/200k, 3/200k, 17/200k, 0, 0 — a worst
	// case of 0.0085%. The bound is more than ten times that and still tight enough
	// to fail if the veto is loosened: at three words the measured rate is 0.28-0.61%,
	// which this catches.
	const (
		samples = 20000
		maxRate = 0.001
	)
	for _, s := range shapes {
		t.Run(s.name, func(t *testing.T) {
			rejected := 0
			for i := 0; i < samples; i++ {
				if wordComposed(synthKey(int64(i), s.n, s.alpha)) {
					rejected++
				}
			}
			if rate := float64(rejected) / samples; rate > maxRate {
				t.Errorf("veto rejected %d of %d generated keys (%.3f%%), over the %.1f%% bound",
					rejected, samples, rate*100, maxRate*100)
			}
		})
	}
}

// TestEntropyCorpus is the measurement itself, kept so it can be re-run rather
// than trusted. It scans a real tree of memories and reports what the entropy
// layer fires on; every hit is a false positive unless that tree genuinely holds
// a credential, which is the number the thresholds were chosen against.
//
// Off by default: it needs a corpus, and there isn't one in the repo. Point it at
// one with ENGRAM_CORPUS_ROOT. Findings print redacted, like everywhere else.
func TestEntropyCorpus(t *testing.T) {
	root := os.Getenv("ENGRAM_CORPUS_ROOT")
	if root == "" {
		t.Skip("set ENGRAM_CORPUS_ROOT to a tree of memories to re-measure the false-positive rate")
	}
	var hits []string
	files, runs := 0, 0
	seen := map[string]bool{}
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".md") {
			return nil //nolint:nilerr // an unreadable corner of the corpus is not a test failure
		}
		b, readErr := os.ReadFile(p)
		if readErr != nil {
			return nil
		}
		files++
		for _, run := range entropyCandidate.FindAllString(string(b), -1) {
			if !seen[run] {
				seen[run] = true
				runs++
			}
		}
		for _, f := range Scan(string(b), ScopeSecrets) {
			if f.Rule == "high-entropy-string" {
				hits = append(hits, fmt.Sprintf("%s:%d %s", filepath.Base(p), f.Line, f.Match))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(hits)
	t.Logf("%d files, %d distinct candidate runs, %d high-entropy findings", files, runs, len(hits))
	for _, h := range hits {
		t.Logf("  %s", h)
	}
}
