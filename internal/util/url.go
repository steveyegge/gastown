package util

import (
	"net/url"
	"strings"
)

// RedactURL strips credentials from a URL for safe logging.
// "https://x-access-token:ghp_abc@github.com/org/repo" → "https://github.com/org/repo"
// SSH-style URLs (git@github.com:org/repo.git) are returned as-is.
// URLs that net/url can't parse but contain no credentials are returned as-is for debugging.
func RedactURL(rawURL string) string {
	// SSH-style URLs and non-standard transports don't use standard URL conventions.
	if !strings.Contains(rawURL, "://") {
		return rawURL
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		// Can't parse — return as-is if no credentials, otherwise mask it.
		if !strings.Contains(rawURL, "@") {
			return rawURL
		}
		return "<invalid URL>"
	}
	u.User = nil
	return u.String()
}
