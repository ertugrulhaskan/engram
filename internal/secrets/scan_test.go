package secrets

import (
	"strings"
	"testing"
)

func TestScan_Secrets(t *testing.T) {
	cases := []struct {
		name, content, wantRule string
	}{
		{"aws", "key = AKIAIOSFODNN7EXAMPLE here", "aws-access-key-id"},
		{"github", "ghp_" + strings.Repeat("a", 36), "github-token"},
		{"github-pat", "github_pat_" + strings.Repeat("b", 30), "github-fine-grained-pat"},
		{"anthropic", "sk-ant-api03-" + strings.Repeat("c", 24), "anthropic-api-key"},
		{"openai-legacy", "sk-" + strings.Repeat("d", 40), "openai-api-key"},
		{"openai-proj", "sk-proj-" + strings.Repeat("d", 24), "openai-api-key"},
		{"stripe", "sk_live_" + strings.Repeat("s", 24), "stripe-secret-key"},
		{"google", "AIza" + strings.Repeat("e", 35), "google-api-key"},
		{"slack", "xoxb-123456789012-abcdefghijkl", "slack-token"},
		{"private-key", "-----BEGIN OPENSSH PRIVATE KEY-----", "private-key-block"},
		{"pgp-private-key", "-----BEGIN PGP PRIVATE KEY BLOCK-----", "private-key-block"},
		{"generic", `password = "hunter2hunter2hunter2"`, "generic-secret-assignment"},
		{"env-secret-key", `CLERK_SECRET_KEY = "randomvalue123456"`, "generic-secret-assignment"},
		{"env-secretkey", `CLERK_SECRETKEY=abcdefghijklmnop`, "generic-secret-assignment"},
		{"env-spaced-key", `NUXT_SECRET_K_E_Y: somevalue123456`, "generic-secret-assignment"},
		{"env-lowercase", `export nuxt_public_secret=abcdefghijklmnop`, "generic-secret-assignment"},
		{"url-creds", `DATABASE_URL=postgres://user:s3cr3tpass@db.example.com/app`, "url-credentials"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fs := Scan(c.content, ScopeSecrets)
			if len(fs) == 0 {
				t.Fatalf("expected a finding for %q, got none", c.content)
			}
			var got string
			for _, f := range fs {
				if f.Rule == c.wantRule {
					got = f.Rule
				}
			}
			if got != c.wantRule {
				t.Errorf("want rule %q, got findings %+v", c.wantRule, fs)
			}
		})
	}
}

func TestScan_DedupesProviderAndGeneric(t *testing.T) {
	// "access_key = AKIA…" matches both the generic keyword rule and the precise
	// aws rule — the generic one must be suppressed so the secret is reported once.
	fs := Scan("access_key = AKIAIOSFODNN7EXAMPLE", ScopeSecrets)
	if len(fs) != 1 || fs[0].Rule != "aws-access-key-id" {
		t.Errorf("want a single aws finding, got %+v", fs)
	}
}

func TestScan_Clean(t *testing.T) {
	content := "---\nname: notes\n---\n# Notes\n\nUse pnpm, not npm. The build runs make lint.\n"
	if fs := Scan(content, ScopeSecretsAndPII); len(fs) != 0 {
		t.Errorf("clean content should have no findings, got %+v", fs)
	}
}

func TestScan_Redacted(t *testing.T) {
	secret := "AKIAIOSFODNN7EXAMPLE"
	fs := Scan("k = "+secret, ScopeSecrets)
	if len(fs) == 0 {
		t.Fatal("expected a finding")
	}
	if strings.Contains(fs[0].Match, secret) {
		t.Errorf("redacted match must not contain the full secret: %q", fs[0].Match)
	}
	if !strings.HasPrefix(fs[0].Match, "AKIA") {
		t.Errorf("redaction should keep a recognizable prefix, got %q", fs[0].Match)
	}
}

func TestScan_PIIScopeGating(t *testing.T) {
	content := "contact jane.doe@example.com for details"
	if fs := Scan(content, ScopeSecrets); len(fs) != 0 {
		t.Errorf("ScopeSecrets must NOT flag an email, got %+v", fs)
	}
	fs := Scan(content, ScopeSecretsAndPII)
	found := false
	for _, f := range fs {
		if f.Rule == "email-address" {
			found = true
		}
	}
	if !found {
		t.Errorf("ScopeSecretsAndPII should flag the email, got %+v", fs)
	}
}

func TestScan_LineNumbers(t *testing.T) {
	content := "line one\nline two\nkey = AKIAIOSFODNN7EXAMPLE\n"
	fs := Scan(content, ScopeSecrets)
	if len(fs) == 0 || fs[0].Line != 3 {
		t.Errorf("expected finding on line 3, got %+v", fs)
	}
}

// TestScanCatchesWrappedSecrets covers ENGR-32: a credential broken across two
// lines matches no rule per line, because no rule ever sees the whole value.
func TestScanCatchesWrappedSecrets(t *testing.T) {
	cases := []struct {
		name    string
		content string
		rule    string
		line    int // the line the match must be reported on: where it starts
	}{
		{
			// A soft-wrapped paste. AKIA needs 16 trailing chars; line 1 has 4.
			name:    "soft wrapped aws key",
			content: "notes\nAKIA1234\n5678ABCDEFGH\ntail",
			rule:    "aws-access-key-id",
			line:    2,
		},
		{
			// A YAML block scalar: the continuation is indented, which must be
			// dropped before the join or the value never reassembles.
			name:    "yaml block scalar",
			content: "api_key: >\n  sk_live_ABCDEFGH\n  IJKLMNOP1234\n",
			rule:    "stripe-secret-key",
			line:    2,
		},
		{
			name:    "backslash continuation",
			content: "export K=AKIA0000\\\n1111222233334444\n",
			rule:    "aws-access-key-id",
			line:    1,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Scan(c.content, ScopeSecrets)
			var hit *Finding
			for i := range got {
				if got[i].Rule == c.rule {
					hit = &got[i]
					break
				}
			}
			if hit == nil {
				t.Fatalf("wrapped secret missed; findings=%+v", got)
			}
			if hit.Line != c.line {
				t.Errorf("reported line %d, want %d (the line the value starts on)", hit.Line, c.line)
			}
			if strings.Contains(hit.Match, "1111222233334444") || strings.Contains(hit.Match, "IJKLMNOP1234") {
				t.Errorf("finding leaked the raw value: %q", hit.Match)
			}
		})
	}
}

// TestScanDoesNotFuseProse guards the cost side of the wrap heuristic: joining
// must not invent a secret out of two ordinary lines, and must not report the
// same secret twice when nothing is wrapped at all.
func TestScanDoesNotFuseProse(t *testing.T) {
	prose := "The deliberately long paragraph continues\nunderstanding that it wraps naturally here\nand keeps going for another line entirely\n"
	if got := Scan(prose, ScopeSecrets); len(got) != 0 {
		t.Errorf("prose must not produce findings, got %+v", got)
	}

	// A markdown bullet list: each line ends and begins with word characters, so
	// a naive joiner would fuse them.
	list := "- alphabetical\n- bravocharlie\n- deltaechofox\n"
	if got := Scan(list, ScopeSecrets); len(got) != 0 {
		t.Errorf("a bullet list must not produce findings, got %+v", got)
	}

	// The no-wrap case: the second pass sees exactly what the first did, so every
	// finding must appear once. This is the property that keeps the new pass from
	// changing established behavior.
	single := "aws_key = AKIA1234567890ABCDEF\n"
	got := Scan(single, ScopeSecrets)
	counts := map[string]int{}
	for _, f := range got {
		counts[f.Rule]++
	}
	for rule, n := range counts {
		if n != 1 {
			t.Errorf("rule %s reported %d times, want 1 (double-report across passes)", rule, n)
		}
	}
	if counts["aws-access-key-id"] != 1 {
		t.Errorf("expected the aws rule to fire once, got %+v", got)
	}
	if counts["generic-secret-assignment"] != 0 {
		t.Error("the generic rule must stay suppressed when a provider rule named the secret")
	}
}

// TestLogicalSegmentsJoinsOnlyWraps pins the wrap heuristic directly. The
// Scan-level prose test can only prove "no findings", which a heuristic that
// fuses everything would also satisfy as long as no rule happened to match —
// this asserts the joining itself.
func TestLogicalSegmentsJoinsOnlyWraps(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    int // logical segments
	}{
		{
			// Both runs are long, but they are English words with no digits.
			name:    "prose does not fuse",
			content: "The deliberately long paragraph continues\nunderstanding that it wraps naturally here",
			want:    2,
		},
		{
			name:    "bullet list does not fuse",
			content: "- alphabetical\n- bravocharlie\n- deltaechofox",
			want:    3,
		},
		{
			name:    "a split token joins",
			content: "AKIA1234\n5678ABCDEFGH",
			want:    1,
		},
		{
			// The header ends in ':' and '>' — not credential characters — so it
			// stays its own segment while the two value lines join.
			name:    "yaml scalar joins its value lines only",
			content: "api_key: >\n  sk_live_ABCDEFGH\n  IJKLMNOP1234",
			want:    2,
		},
		{
			name:    "explicit continuation always joins",
			content: `short\` + "\nnext",
			want:    1,
		},
		{
			name:    "a blank line never joins",
			content: "AKIA1234\n\n5678ABCDEFGH",
			want:    3,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := logicalSegments(c.content)
			if len(got) != c.want {
				t.Errorf("got %d segments, want %d: %+v", len(got), c.want, got)
			}
		})
	}
}

// TestWrapBoundaryIsMultibyteSafe pins a non-obvious invariant: tailRun and
// headRun count runes, while logicalSegments slices the result by byte offset.
// That is only correct because isWrapChar accepts ASCII alone, so a credential
// run's rune count always equals its byte length. If the alphabet ever grows a
// non-ASCII member, this test is what fails.
func TestWrapBoundaryIsMultibyteSafe(t *testing.T) {
	content := "日本語のメモ AKIA1234\n5678ABCDEFGH"
	if segs := logicalSegments(content); len(segs) != 1 {
		t.Fatalf("multibyte text before the run broke the join: %+v", segs)
	}
	var found bool
	for _, f := range Scan(content, ScopeSecrets) {
		if f.Rule == "aws-access-key-id" {
			found = true
		}
	}
	if !found {
		t.Error("a wrapped key following multibyte text was missed")
	}
	if segs := logicalSegments("🔑AKIA1234\n5678ABCDEFGH"); len(segs) != 1 {
		t.Errorf("emoji directly before the run broke the join: %+v", segs)
	}
}
