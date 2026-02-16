package github

import "testing"

func TestParseGitHubRepo_HTTPS(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		want    string
		wantErr bool
	}{
		{
			name: "https with .git",
			url:  "https://github.com/steveyegge/gastown.git",
			want: "steveyegge/gastown",
		},
		{
			name: "https without .git",
			url:  "https://github.com/steveyegge/gastown",
			want: "steveyegge/gastown",
		},
		{
			name: "ssh format",
			url:  "git@github.com:steveyegge/gastown.git",
			want: "steveyegge/gastown",
		},
		{
			name: "ssh without .git",
			url:  "git@github.com:steveyegge/gastown",
			want: "steveyegge/gastown",
		},
		{
			name:    "not github",
			url:     "https://gitlab.com/user/repo.git",
			wantErr: true,
		},
		{
			name:    "empty",
			url:     "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseGitHubRepo(tt.url)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseGitHubRepo(%q) expected error, got %q", tt.url, got)
				}
				return
			}
			if err != nil {
				t.Errorf("ParseGitHubRepo(%q) error = %v", tt.url, err)
				return
			}
			if got != tt.want {
				t.Errorf("ParseGitHubRepo(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}

func TestCheckRun_IsFailed(t *testing.T) {
	tests := []struct {
		conclusion string
		want       bool
	}{
		{"failure", true},
		{"timed_out", true},
		{"canceled", true},
		{"action_required", true},
		{"success", false},
		{"neutral", false},
		{"skipped", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.conclusion, func(t *testing.T) {
			c := CheckRun{Conclusion: tt.conclusion}
			if got := c.IsFailed(); got != tt.want {
				t.Errorf("CheckRun{Conclusion: %q}.IsFailed() = %v, want %v",
					tt.conclusion, got, tt.want)
			}
		})
	}
}
