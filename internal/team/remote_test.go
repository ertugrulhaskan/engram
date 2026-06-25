package team

import "testing"

func TestNormalizeRemote(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"scp with .git", "git@github.com:acme/app.git", "github.com/acme/app"},
		{"scp without .git", "git@github.com:acme/app", "github.com/acme/app"},
		{"scp bare host (no user)", "github.com:acme/app.git", "github.com/acme/app"},
		{"scp userinfo with colon", "user:pass@github.com:acme/app.git", "github.com/acme/app"},
		{"https userinfo with colon", "https://user:pass@github.com/acme/app.git", "github.com/acme/app"},
		{"https with .git", "https://github.com/acme/app.git", "github.com/acme/app"},
		{"https without .git", "https://github.com/acme/app", "github.com/acme/app"},
		{"https trailing slash", "https://github.com/acme/app/", "github.com/acme/app"},
		{"ssh with user", "ssh://git@github.com/acme/app.git", "github.com/acme/app"},
		{"ssh with port", "ssh://git@github.com:2222/acme/app.git", "github.com/acme/app"},
		{"git scheme", "git://github.com/acme/app.git", "github.com/acme/app"},
		{"mixed case lowercased", "https://GitHub.com/Acme/App.git", "github.com/acme/app"},
		{"nested path", "git@github.com:team/sub/app.git", "github.com/team/sub/app"},
		{"surrounding whitespace", "  https://github.com/acme/app.git  ", "github.com/acme/app"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeRemote(tt.in)
			if err != nil {
				t.Fatalf("NormalizeRemote(%q) returned error: %v", tt.in, err)
			}
			if got != tt.want {
				t.Errorf("NormalizeRemote(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestNormalizeRemoteErrors(t *testing.T) {
	for _, in := range []string{
		"",
		"   ",
		"not-a-url",
		"https://github.com/", // no path
		"https://github.com",  // no path
	} {
		if got, err := NormalizeRemote(in); err == nil {
			t.Errorf("NormalizeRemote(%q) = %q, want error", in, got)
		}
	}
}
