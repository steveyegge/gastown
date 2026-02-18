package util

import "testing"

func TestRedactURL(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"no credentials", "https://github.com/org/repo", "https://github.com/org/repo"},
		{"user info", "https://user:pass@github.com/org/repo", "https://github.com/org/repo"},
		{"token auth", "https://x-access-token:ghp_abc123@github.com/org/repo.git", "https://github.com/org/repo.git"},
		{"user only", "https://user@github.com/org/repo", "https://github.com/org/repo"},
		{"ssh url", "git@github.com:org/repo.git", "git@github.com:org/repo.git"},
		{"empty string", "", ""},
		{"invalid url no creds", "://bad", "://bad"},
		{"invalid url with creds", "://user:pass@host", "<invalid URL>"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RedactURL(tt.in)
			if got != tt.want {
				t.Errorf("RedactURL(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
