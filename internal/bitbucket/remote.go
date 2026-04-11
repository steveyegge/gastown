package bitbucket

import (
	"fmt"
	"strings"
)

// ParseBitbucketRemote extracts workspace and repo_slug from a Bitbucket git remote URL.
// Supports HTTPS (https://bitbucket.org/workspace/repo.git) and
// SSH (git@bitbucket.org:workspace/repo.git) formats.
func ParseBitbucketRemote(remoteURL string) (workspace, repoSlug string, err error) {
	var path string
	switch {
	case strings.HasPrefix(remoteURL, "https://bitbucket.org/"):
		path = strings.TrimPrefix(remoteURL, "https://bitbucket.org/")
	case strings.HasPrefix(remoteURL, "git@bitbucket.org:"):
		path = strings.TrimPrefix(remoteURL, "git@bitbucket.org:")
	default:
		return "", "", fmt.Errorf("bitbucket: not a Bitbucket URL: %s", remoteURL)
	}

	path = strings.TrimSuffix(path, ".git")
	path = strings.TrimSuffix(path, "/")

	parts := strings.SplitN(path, "/", 3)
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("bitbucket: cannot parse workspace/repo from: %s", remoteURL)
	}
	return parts[0], parts[1], nil
}
