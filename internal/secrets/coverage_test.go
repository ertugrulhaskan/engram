package secrets

import (
	"strings"
	"testing"
)

// Credentials the scanner used to miss, so the promote guard waved them into a
// shared git repo where a leak is effectively permanent. Each is a common real
// shape: a password with symbols or under 12 characters (the generic rule's
// value class was base64-only, 12+), a key named with a keyword it lacked
// (PASS, PWD, CREDENTIAL), or a provider token with no rule of its own.
func TestScanCatchesFormerlyMissedCredentials(t *testing.T) {
	cases := []struct{ name, content string }{
		{"password with symbols", `DB_PASSWORD=P@ssw0rd!2024xyz`},
		{"short password with symbol", `PASSWORD=Summer2024!`},
		{"quoted yaml password", `password: "P@ssw0rd123"`},
		{"symbol early in value", `password: Hunter2!Hunter2!Hunter2`},
		{"token with dollar", `export TOKEN="ab12$cd34ef56gh78"`},
		{"PASS keyword", `DB_PASS=Tr0ub4dor3xyzabc`},
		{"pwd keyword", `pwd=Zx9fQk2LmN8vTr4Y`},
		{"credential keyword", `smtp_credential: Xk3!pQ9#mW`},
		{"gitlab pat in prose", "the deploy uses glpat-" + strings.Repeat("Ab3_", 5) + " for CI"},
		{"shopify", "shpat_" + strings.Repeat("0a1b2c3d", 4)},
		{"digitalocean", "dop_v1_" + strings.Repeat("0123456789abcdef", 4)},
		{"mailgun", "key-" + strings.Repeat("0123456789abcdef", 2)},
		{"twilio api key", "SK" + strings.Repeat("0123456789ABCDEF", 2)},
		{"hugging face", "hf_" + strings.Repeat("AbCdEfGhIjKlMnOpQ", 2)},
		{"npm", "npm_" + strings.Repeat("A1b2C3d4E5f6", 3)},
		// Weak but real values the guard has always reported — the reference
		// filter must not swallow them for merely containing a secret word.
		{"lowercase password with the word in it", `password=mysecretpassword`},
		{"dashed token with the word in it", `API_TOKEN=my-token-value-here`},
		// Passwords the first reference filter swallowed by substring.
		{"contains your", `DB_PASSWORD=Yourbaby123!`},
		{"contains parentheses", `PASSWORD=P@ss(word)1x`},
		{"contains example", `password: MyExample#Pass9`},
		{"starts with percent", `password=%Secret123!x`},
		// A real secret after a reference on the same line.
		{"secret after a reference", `DB_PASSWORD=DB_PASSWORD_FROM_VAULT ADMIN_SECRET=correcthorsebatterystaple`},
		// camelCase values the old rule reported must still be reported.
		{"camelCase password", `password: MySecretPassword`},
		{"camelCase api key", `API_KEY=NextApiKeyValue`},
		// Markdown bold keys, the way memories often write them.
		{"bold key, colon inside", `**Password:** Summer2024!`},
		{"bold key, colon outside", `**DB password**: Tr0ub4dor&3`},
		{"braces inside a password", `password: Pa$$w0rd}}x`},
		// Short real passwords.
		{"seven characters", `password: Abc123!`},
		{"short DB_PASS", `DB_PASS=Hunt3r!`},
		{"short, bold key", `**Password:** s3cr3t#1`},
		// Dots, parentheses or an env-ish word inside a real password.
		{"dot and parens inside", `password: P@ss.word(1)x`},
		{"call-like inside", `password: Abc.def(1)!`},
		{"getenv inside", `password: Mygetenv123!`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if fs := Scan(c.content, ScopeSecrets); len(fs) == 0 {
				t.Errorf("no finding for %q", c.content)
			}
		})
	}
}

// What a memory says ABOUT secrets must stay clean, or the guard cries wolf and
// people learn to press the override by habit — which is worse than a miss.
// Config references, code, types, placeholders, prose, and hashes.
func TestScanLeavesSecretTalkAlone(t *testing.T) {
	clean := []string{
		`password: ${DB_PASSWORD}`,
		`token: process.env.GITHUB_TOKEN`,
		`api_key = os.environ["OPENAI_API_KEY"]`,
		`password: z.string().min(8)`,
		`password: Optional[str] = None`,
		`token: <your-token-here>`,
		`secret: {{ .Values.secret }}`,
		`password: required`,
		`token: string | null`,
		`password: newPassword`,
		`DB_PASSWORD=DB_PASSWORD_FROM_VAULT`,
		`password=******** (redacted)`,
		`token: xxxxxxxxxxxxxxxx`,
		`The password must be at least 12 characters and contain a symbol.`,
		`bypass=enabled-for-tests`,
		`passport: renewed-2026`,
		`commit 3f2a9c1d8e7b6a5f4e3d2c1b0a9f8e7d6c5b4a39 fixed the token refresh`,
		`sha256 0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef`,
		`md5 9e107d9d372bb6826bd81d3542a419d6`,
		`see https://docs.example.com/auth/tokens for how rotation works`,
		// Code inside memories — the two shapes a measurement against a real tree
		// of memories flagged before the trailing-punctuation and reference checks.
		`  'x-api-key': config.widgetsApiKey,`,
		`  fetchItems(tokens: TokenSet, projectId: string): Promise<Item[]>`,
		`  refresh(tokens: TokenSet): Promise<TokenSet>`,
		`token = getSecret()`,
		`password: fetchPassword()`,
	}
	for _, c := range clean {
		if fs := Scan(c, ScopeSecrets); len(fs) != 0 {
			t.Errorf("false positive on %q: %+v", c, fs)
		}
	}
}
