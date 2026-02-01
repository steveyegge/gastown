package cmd

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/steveyegge/gastown/internal/style"
)

type forkDecision struct {
	OriginURL string
	Forked    bool
}

func resolveForkOrigin(upstreamURL, originURL, policy string, yes bool) (forkDecision, error) {
	if originURL != "" && originURL != upstreamURL {
		return forkDecision{OriginURL: originURL, Forked: true}, nil
	}
	if upstreamURL == "" {
		return forkDecision{OriginURL: originURL, Forked: false}, nil
	}
	if policy == "never" {
		return forkDecision{OriginURL: upstreamURL, Forked: false}, nil
	}

	owner, repo, ok := parseGitHubRepo(upstreamURL)
	if !ok {
		return forkDecision{OriginURL: upstreamURL, Forked: false}, nil
	}

	if _, err := exec.LookPath("gh"); err != nil {
		fmt.Printf("  %s GitHub CLI (gh) not found, using upstream\n", style.Warning.Render("!"))
		return forkDecision{OriginURL: upstreamURL, Forked: false}, nil
	}

	username, err := getGitHubUsername()
	if err != nil || username == "" {
		fmt.Printf("  %s Could not determine GitHub user, using upstream\n", style.Warning.Render("!"))
		return forkDecision{OriginURL: upstreamURL, Forked: false}, nil
	}
	if strings.EqualFold(username, owner) {
		return forkDecision{OriginURL: upstreamURL, Forked: false}, nil
	}

	forkURL, exists := lookupForkURL(username, repo)
	if exists {
		if policy == "require" || yes {
			return forkDecision{OriginURL: forkURL, Forked: true}, nil
		}
		if promptYesNo(fmt.Sprintf("Use existing fork %s/%s as origin?", username, repo)) {
			return forkDecision{OriginURL: forkURL, Forked: true}, nil
		}
		return forkDecision{OriginURL: upstreamURL, Forked: false}, nil
	}

	if policy == "require" {
		if err := createFork(upstreamURL); err != nil {
			return forkDecision{OriginURL: upstreamURL, Forked: false}, err
		}
		if forkURL, exists := lookupForkURL(username, repo); exists {
			return forkDecision{OriginURL: forkURL, Forked: true}, nil
		}
		return forkDecision{OriginURL: upstreamURL, Forked: false}, fmt.Errorf("fork created but could not resolve URL")
	}

	if policy == "prompt" && !yes {
		if promptYesNo(fmt.Sprintf("Create fork %s/%s with gh?", username, repo)) {
			if err := createFork(upstreamURL); err != nil {
				return forkDecision{OriginURL: upstreamURL, Forked: false}, err
			}
			if forkURL, exists := lookupForkURL(username, repo); exists {
				return forkDecision{OriginURL: forkURL, Forked: true}, nil
			}
			return forkDecision{OriginURL: upstreamURL, Forked: false}, fmt.Errorf("fork created but could not resolve URL")
		}
	}

	return forkDecision{OriginURL: upstreamURL, Forked: false}, nil
}

func getGitHubUsername() (string, error) {
	cmd := exec.Command("gh", "api", "user", "--jq", ".login")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func lookupForkURL(user, repo string) (string, bool) {
	full := fmt.Sprintf("%s/%s", user, repo)
	cmd := exec.Command("gh", "repo", "view", full, "--json", "url", "--jq", ".url")
	out, err := cmd.Output()
	if err != nil {
		return "", false
	}
	url := strings.TrimSpace(string(out))
	if url == "" {
		return "", false
	}
	return url, true
}

func createFork(upstreamURL string) error {
	cmd := exec.Command("gh", "repo", "fork", upstreamURL, "--clone=false")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("gh repo fork failed: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}
