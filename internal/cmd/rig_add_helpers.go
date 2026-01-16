package cmd

import (
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type rigPreset struct {
	Name          string
	UpstreamURL   string
	SetupCommand  string
	SetupWorkdir  string
	DefaultBranch string
	Prefix        string
}

var rigPresets = map[string]rigPreset{
	"gastown": {
		Name:          "gastown",
		UpstreamURL:   "https://github.com/steveyegge/gastown.git",
		SetupCommand:  "go install ./cmd/gt",
		SetupWorkdir:  ".",
		DefaultBranch: "main",
		Prefix:        "gt",
	},
	"beads": {
		Name:          "beads",
		UpstreamURL:   "https://github.com/steveyegge/beads.git",
		SetupCommand:  "go install ./cmd/bd",
		SetupWorkdir:  ".",
		DefaultBranch: "main",
		Prefix:        "bd",
	},
}

func isLocalPath(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func isLikelyGitURL(input string) bool {
	if strings.Contains(input, "://") {
		return true
	}
	if strings.HasPrefix(input, "git@") {
		return true
	}
	if strings.HasPrefix(input, "ssh://") {
		return true
	}
	if strings.HasSuffix(input, ".git") {
		return true
	}
	return false
}

func sanitizeRigName(name string) string {
	name = strings.ReplaceAll(name, "-", "_")
	name = strings.ReplaceAll(name, ".", "_")
	name = strings.ReplaceAll(name, " ", "_")
	return name
}

func deriveRigNameFromURL(input string) string {
	clean := strings.TrimSuffix(strings.TrimSpace(input), ".git")
	clean = strings.TrimSuffix(clean, "/")
	if idx := strings.LastIndex(clean, ":"); idx != -1 && strings.Contains(clean[:idx], "@") {
		clean = clean[idx+1:]
	}
	base := filepath.Base(clean)
	return sanitizeRigName(base)
}

func deriveRigNameFromPath(path string) string {
	return sanitizeRigName(filepath.Base(path))
}

func normalizeForkPolicy(policy string) (string, error) {
	if policy == "" {
		return "prompt", nil
	}
	switch policy {
	case "prompt", "require", "never":
		return policy, nil
	default:
		return "", fmt.Errorf("invalid fork policy %q (use prompt, require, or never)", policy)
	}
}

func parseGitHubRepo(raw string) (string, string, bool) {
	clean := strings.TrimSuffix(strings.TrimSpace(raw), ".git")

	if strings.HasPrefix(clean, "http://") || strings.HasPrefix(clean, "https://") || strings.HasPrefix(clean, "ssh://") {
		u, err := url.Parse(clean)
		if err != nil {
			return "", "", false
		}
		host := u.Host
		if idx := strings.Index(host, "@"); idx != -1 {
			host = host[idx+1:]
		}
		if !strings.EqualFold(host, "github.com") {
			return "", "", false
		}
		parts := strings.Split(strings.TrimPrefix(u.Path, "/"), "/")
		if len(parts) < 2 {
			return "", "", false
		}
		return parts[0], parts[1], true
	}

	if strings.HasPrefix(clean, "git@github.com:") {
		rest := strings.TrimPrefix(clean, "git@github.com:")
		parts := strings.Split(rest, "/")
		if len(parts) < 2 {
			return "", "", false
		}
		return parts[0], parts[1], true
	}

	if strings.HasPrefix(clean, "git://github.com/") {
		rest := strings.TrimPrefix(clean, "git://github.com/")
		parts := strings.Split(rest, "/")
		if len(parts) < 2 {
			return "", "", false
		}
		return parts[0], parts[1], true
	}

	return "", "", false
}

func findGitRoot(path string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = path
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func findGitRemoteURL(gitRoot string) (string, error) {
	cmd := exec.Command("git", "remote", "get-url", "origin")
	cmd.Dir = gitRoot
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
