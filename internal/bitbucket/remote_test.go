package bitbucket

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseBitbucketRemote(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		url       string
		workspace string
		repoSlug  string
		wantErr   bool
	}{
		{
			name:      "HTTPS with .git suffix",
			url:       "https://bitbucket.org/myworkspace/myrepo.git",
			workspace: "myworkspace",
			repoSlug:  "myrepo",
		},
		{
			name:      "HTTPS without .git suffix",
			url:       "https://bitbucket.org/myworkspace/myrepo",
			workspace: "myworkspace",
			repoSlug:  "myrepo",
		},
		{
			name:      "HTTPS with trailing slash",
			url:       "https://bitbucket.org/myworkspace/myrepo/",
			workspace: "myworkspace",
			repoSlug:  "myrepo",
		},
		{
			name:      "SSH format",
			url:       "git@bitbucket.org:myworkspace/myrepo.git",
			workspace: "myworkspace",
			repoSlug:  "myrepo",
		},
		{
			name:      "SSH without .git suffix",
			url:       "git@bitbucket.org:myworkspace/myrepo",
			workspace: "myworkspace",
			repoSlug:  "myrepo",
		},
		{
			name:    "GitHub URL returns error",
			url:     "https://github.com/owner/repo",
			wantErr: true,
		},
		{
			name:    "Empty URL returns error",
			url:     "",
			wantErr: true,
		},
		{
			name:    "Missing repo returns error",
			url:     "https://bitbucket.org/myworkspace",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ws, repo, err := ParseBitbucketRemote(tt.url)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.workspace, ws)
			assert.Equal(t, tt.repoSlug, repo)
		})
	}
}
