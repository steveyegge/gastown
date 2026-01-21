package cmd

import (
	"fmt"
	"os/exec"
	"runtime/debug"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/version"
)

// Version information - set at build time via ldflags
var (
	Version = "0.4.0"
	// Build can be set via ldflags at compile time
	Build = "dev"
	// Commit and Branch - the git revision the binary was built from (optional ldflag)
	Commit = ""
	Branch = ""
)

var versionCmd = &cobra.Command{
	Use:     "version",
	GroupID: GroupDiag,
	Short:   "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		commit := resolveCommitHash()
		branch := resolveBranch()

		if commit != "" && branch != "" {
			fmt.Printf("gt version %s (%s: %s@%s)\n", Version, Build, branch, version.ShortCommit(commit))
		} else if commit != "" {
			fmt.Printf("gt version %s (%s: %s)\n", Version, Build, version.ShortCommit(commit))
		} else {
			fmt.Printf("gt version %s (%s)\n", Version, Build)
		}
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)

	// Pass the build-time commit to the version package for stale binary checks
	if Commit != "" {
		version.SetCommit(Commit)
	}
}

func resolveCommitHash() string {
	if Commit != "" {
		return Commit
	}

	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" && setting.Value != "" {
				return setting.Value
			}
		}
	}

	return ""
}

func resolveBranch() string {
	if Branch != "" {
		return Branch
	}

	// Try to get branch from build info (build-time VCS detection)
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			if setting.Key == "vcs.branch" && setting.Value != "" {
				return setting.Value
			}
		}
	}

	// Fallback: try to get branch from git at runtime
	cmd := exec.Command("git", "symbolic-ref", "--short", "HEAD")
	cmd.Dir = "."
	if output, err := cmd.Output(); err == nil {
		if branch := strings.TrimSpace(string(output)); branch != "" && branch != "HEAD" {
			return branch
		}
	}

	return ""
}
