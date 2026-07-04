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
