package cmd

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/steveyegge/gastown/internal/git"
)

func configureRigRemotes(rigPath, originURL, upstreamURL string) error {
	repos := []struct {
		name string
		git  *git.Git
	}{
		{
			name: "bare",
			git:  git.NewGitWithDir(filepath.Join(rigPath, ".repo.git"), ""),
		},
		{
			name: "mayor",
			git:  git.NewGit(filepath.Join(rigPath, "mayor", "rig")),
		},
		{
			name: "refinery",
			git:  git.NewGit(filepath.Join(rigPath, "refinery", "rig")),
		},
	}

	for _, repo := range repos {
		if err := repo.git.SetRemoteURL("origin", originURL); err != nil {
			return fmt.Errorf("%s: set origin: %w", repo.name, err)
		}
		if upstreamURL != "" && upstreamURL != originURL {
			if err := repo.git.SetRemoteURL("upstream", upstreamURL); err != nil {
				return fmt.Errorf("%s: set upstream: %w", repo.name, err)
			}
		}
	}

	return nil
}

func setBeadsSyncRemote(rigPath, remote string) error {
	cmd := exec.Command("bd", "config", "set", "sync.remote", remote)
	cmd.Dir = rigPath
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("bd config set sync.remote failed: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}
