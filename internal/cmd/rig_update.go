package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/rig"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

var (
	rigUpdateCheck          bool
	rigUpdatePull           bool
	rigUpdateIgnoreManifest bool
)

var rigUpdateCmd = &cobra.Command{
	Use:   "update [rig]",
	Short: "Check or update a rig against upstream/origin",
	Long: `Check or update a rig against upstream/origin.

If no rig is specified, the rig is inferred from the current directory.

Examples:
  gt rig update gastown --check
  gt rig update gastown --pull`,
	Args: cobra.MaximumNArgs(1),
	RunE: runRigUpdate,
}

func init() {
	rigCmd.AddCommand(rigUpdateCmd)
	rigUpdateCmd.Flags().BoolVar(&rigUpdateCheck, "check", false, "Check for updates without pulling")
	rigUpdateCmd.Flags().BoolVar(&rigUpdatePull, "pull", false, "Pull updates into mayor/refinery clones")
	rigUpdateCmd.Flags().BoolVar(&rigUpdateIgnoreManifest, "ignore-manifest", false, "Ignore .gt/rig.toml defaults")
}

func runRigUpdate(cmd *cobra.Command, args []string) error {
	rigName := ""
	if len(args) > 0 {
		rigName = args[0]
	}

	if rigName == "" {
		townRoot, err := workspace.FindFromCwdOrError()
		if err != nil {
			return fmt.Errorf("not in a Gas Town workspace: %w", err)
		}
		inferred, err := inferRigFromCwd(townRoot)
		if err != nil {
			return fmt.Errorf("could not determine rig (use rig name): %w", err)
		}
		rigName = inferred
	}

	check := rigUpdateCheck
	pull := rigUpdatePull
	if !check && !pull {
		check = true
	}

	_, r, err := getRig(rigName)
	if err != nil {
		return err
	}

	rigCfg, err := rig.LoadRigConfig(r.Path)
	if err != nil {
		return fmt.Errorf("loading rig config: %w", err)
	}
	defaultBranch := rigCfg.DefaultBranch
	if defaultBranch == "" {
		defaultBranch = "main"
	}

	originURL := r.OriginURL
	if originURL == "" {
		originURL = r.GitURL
	}
	upstreamURL := r.UpstreamURL

	if err := configureRigRemotes(r.Path, originURL, upstreamURL); err != nil {
		fmt.Printf("  %s Could not configure remotes: %v\n", style.Warning.Render("!"), err)
	}

	remoteName := "origin"
	if upstreamURL != "" && upstreamURL != originURL {
		remoteName = "upstream"
	}

	if check {
		if err := reportRigStatus(r, remoteName, defaultBranch); err != nil {
			return err
		}
	}

	if pull {
		if err := pullRigRepos(r, remoteName, defaultBranch); err != nil {
			return err
		}
		if !rigUpdateIgnoreManifest {
			if err := runRigUpdateSetup(r, rigCfg); err != nil {
				return err
			}
		}
	}

	return nil
}

type repoStatus struct {
	CurrentBranch string
	Ahead         int
	Behind        int
}

func reportRigStatus(r *rig.Rig, remoteName, branch string) error {
	mayorPath := filepath.Join(r.Path, "mayor", "rig")
	status, err := checkRepoStatus(mayorPath, remoteName, branch)
	if err != nil {
		return err
	}

	fmt.Printf("Rig %s (%s/%s):\n", r.Name, remoteName, branch)
	if status.CurrentBranch != "" && status.CurrentBranch != branch {
		fmt.Printf("  %s mayor is on %s (expected %s)\n", style.Warning.Render("!"), status.CurrentBranch, branch)
	}
	if status.Ahead == 0 && status.Behind == 0 {
		fmt.Printf("  %s Up to date\n", style.Success.Render("OK"))
		return nil
	}
	fmt.Printf("  Ahead %d, behind %d\n", status.Ahead, status.Behind)
	return nil
}

func checkRepoStatus(repoPath, remoteName, branch string) (repoStatus, error) {
	g := git.NewGit(repoPath)
	if err := g.Fetch(remoteName); err != nil {
		return repoStatus{}, fmt.Errorf("fetching %s: %w", remoteName, err)
	}
	currentBranch, err := g.CurrentBranch()
	if err != nil {
		currentBranch = ""
	}
	remoteRef := fmt.Sprintf("%s/%s", remoteName, branch)
	ahead, behind, err := g.AheadBehind(branch, remoteRef)
	if err != nil {
		return repoStatus{}, fmt.Errorf("checking %s: %w", remoteRef, err)
	}
	return repoStatus{CurrentBranch: currentBranch, Ahead: ahead, Behind: behind}, nil
}

type pullResult struct {
	Updated bool
	Skipped bool
	Reason  string
	Behind  int
}

func pullRigRepos(r *rig.Rig, remoteName, branch string) error {
	repos := []struct {
		label string
		path  string
	}{
		{"mayor", filepath.Join(r.Path, "mayor", "rig")},
		{"refinery", filepath.Join(r.Path, "refinery", "rig")},
	}

	fmt.Printf("Updating %s (%s/%s):\n", r.Name, remoteName, branch)
	for _, repo := range repos {
		result, err := pullRigRepo(repo.label, repo.path, remoteName, branch)
		if err != nil {
			return err
		}
		switch {
		case result.Skipped:
			fmt.Printf("  %s %s: %s\n", style.Warning.Render("!"), repo.label, result.Reason)
		case result.Updated:
			fmt.Printf("  %s %s: pulled %d commits\n", style.Success.Render("OK"), repo.label, result.Behind)
		default:
			fmt.Printf("  %s %s: up to date\n", style.Dim.Render("-"), repo.label)
		}
	}

	return nil
}

func pullRigRepo(label, repoPath, remoteName, branch string) (pullResult, error) {
	if _, err := os.Stat(repoPath); err != nil {
		if os.IsNotExist(err) {
			return pullResult{Skipped: true, Reason: "missing repo"}, nil
		}
		return pullResult{}, fmt.Errorf("%s: stat repo: %w", label, err)
	}

	g := git.NewGit(repoPath)
	currentBranch, err := g.CurrentBranch()
	if err != nil {
		return pullResult{}, fmt.Errorf("%s: current branch: %w", label, err)
	}
	if currentBranch != branch {
		return pullResult{Skipped: true, Reason: fmt.Sprintf("on %s (expected %s)", currentBranch, branch)}, nil
	}

	hasChanges, err := g.HasUncommittedChanges()
	if err != nil {
		return pullResult{}, fmt.Errorf("%s: status: %w", label, err)
	}
	if hasChanges {
		return pullResult{Skipped: true, Reason: "has uncommitted changes"}, nil
	}

	if err := g.Fetch(remoteName); err != nil {
		return pullResult{}, fmt.Errorf("%s: fetch %s: %w", label, remoteName, err)
	}
	remoteRef := fmt.Sprintf("%s/%s", remoteName, branch)
	ahead, behind, err := g.AheadBehind(branch, remoteRef)
	if err != nil {
		return pullResult{}, fmt.Errorf("%s: compare %s: %w", label, remoteRef, err)
	}
	if ahead > 0 {
		return pullResult{Skipped: true, Reason: fmt.Sprintf("ahead by %d commits", ahead)}, nil
	}
	if behind == 0 {
		return pullResult{}, nil
	}

	if err := g.PullFastForward(remoteName, branch); err != nil {
		return pullResult{}, fmt.Errorf("%s: pull: %w", label, err)
	}

	return pullResult{Updated: true, Behind: behind}, nil
}

func runRigUpdateSetup(r *rig.Rig, rigCfg *rig.RigConfig) error {
	repoRoot := filepath.Join(r.Path, "mayor", "rig")

	setupCommand := ""
	setupWorkdir := ""

	if manifest, err := rig.LoadManifest(repoRoot); err != nil {
		fmt.Printf("  %s Could not load rig manifest: %v\n", style.Warning.Render("!"), err)
	} else if manifest != nil {
		setupCommand = manifest.Setup.Command
		setupWorkdir = manifest.Setup.Workdir
	}

	if setupCommand == "" && rigCfg != nil && rigCfg.Setup != nil {
		setupCommand = rigCfg.Setup.Command
		setupWorkdir = rigCfg.Setup.Workdir
	}

	if setupCommand == "" {
		return nil
	}

	fmt.Printf("Running setup: %s\n", setupCommand)
	return runRigSetupCommand(repoRoot, setupCommand, setupWorkdir)
}
