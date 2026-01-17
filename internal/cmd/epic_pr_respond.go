package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

var (
	epicPRRespondCrew string
)

var epicPRRespondCmd = &cobra.Command{
	Use:   "respond <pr-number>",
	Short: "Address review feedback on a PR",
	Long: `Address review feedback on an upstream PR.

This command:
1. Fetches review comments from the PR
2. Shows comments to the operator
3. Creates a work bead for addressing feedback
4. Optionally slings to crew member for response
5. When done, push updated branch and comment on PR

EXAMPLES:

  gt epic pr respond 102
  gt epic pr respond 102 --crew dave`,
	Args: cobra.ExactArgs(1),
	RunE: runEpicPRRespond,
}

func init() {
	epicPRRespondCmd.Flags().StringVar(&epicPRRespondCrew, "crew", "", "Crew member to sling work to")
}

func runEpicPRRespond(cmd *cobra.Command, args []string) error {
	prNumStr := args[0]
	var prNum int
	if _, err := fmt.Sscanf(prNumStr, "%d", &prNum); err != nil {
		return fmt.Errorf("invalid PR number: %s", prNumStr)
	}

	// Find town root and detect rig from cwd
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return err
	}

	// Detect rig from current directory
	cwd, _ := os.Getwd()
	rigName := epicDetectRigFromPath(cwd, townRoot)
	if rigName == "" {
		return fmt.Errorf("could not detect rig from current directory")
	}

	rigPath := filepath.Join(townRoot, rigName)
	repoDir := filepath.Join(rigPath, "mayor", "rig")

	// Fetch PR reviews and comments
	fmt.Printf("%s Fetching reviews for PR #%d...\n\n", style.Bold.Render("🔍"), prNum)

	reviews, err := getPRReviews(repoDir, prNum)
	if err != nil {
		return fmt.Errorf("fetching reviews: %w", err)
	}

	comments, err := getPRComments(repoDir, prNum)
	if err != nil {
		return fmt.Errorf("fetching comments: %w", err)
	}

	if len(reviews) == 0 && len(comments) == 0 {
		fmt.Println("No reviews or comments found on this PR.")
		return nil
	}

	// Display reviews
	if len(reviews) > 0 {
		fmt.Printf("%s\n\n", style.Bold.Render("Reviews:"))
		for _, r := range reviews {
			stateIcon := "○"
			switch r.State {
			case "APPROVED":
				stateIcon = style.Success.Render("✓")
			case "CHANGES_REQUESTED":
				stateIcon = style.Warning.Render("!")
			case "COMMENTED":
				stateIcon = "💬"
			}

			fmt.Printf("  %s %s (@%s)\n", stateIcon, r.State, r.Author)
			if r.Body != "" {
				// Indent the body
				for _, line := range strings.Split(r.Body, "\n") {
					if line != "" {
						fmt.Printf("    %s\n", line)
					}
				}
			}
			fmt.Println()
		}
	}

	// Display comments
	if len(comments) > 0 {
		fmt.Printf("%s\n\n", style.Bold.Render("Comments:"))
		for _, c := range comments {
			location := ""
			if c.Path != "" {
				location = fmt.Sprintf(" on %s", c.Path)
				if c.Line > 0 {
					location += fmt.Sprintf(":%d", c.Line)
				}
			}

			fmt.Printf("  💬 @%s%s:\n", c.Author, location)
			// Indent the body
			for _, line := range strings.Split(c.Body, "\n") {
				if line != "" {
					fmt.Printf("    %s\n", line)
				}
			}
			fmt.Println()
		}
	}

	// Count actionable items
	changesRequested := 0
	for _, r := range reviews {
		if r.State == "CHANGES_REQUESTED" {
			changesRequested++
		}
	}

	if changesRequested > 0 {
		fmt.Printf("%s %d review(s) requesting changes\n\n", style.Warning.Render("⚠"), changesRequested)
	}

	// Prompt for action
	fmt.Println("Options:")
	fmt.Println("  1. Address feedback manually (checkout branch and make changes)")
	fmt.Println("  2. Create work bead and sling to crew member")
	fmt.Println("  3. Exit")
	fmt.Print("\nChoice [1]: ")

	var choice string
	fmt.Scanln(&choice)
	choice = strings.TrimSpace(choice)

	switch choice {
	case "", "1":
		// Manual - just show instructions
		fmt.Println()
		fmt.Println("To address feedback:")
		fmt.Printf("  1. Checkout the PR branch: gh pr checkout %d\n", prNum)
		fmt.Println("  2. Make changes to address the feedback")
		fmt.Println("  3. Commit and push: git push")
		fmt.Printf("  4. Optionally reply to comments: gh pr comment %d -b \"Addressed feedback\"\n", prNum)

	case "2":
		// Create work bead
		crewMember := epicPRRespondCrew
		if crewMember == "" {
			// Prompt for crew member
			members := listCrewMembers(rigPath)
			if len(members) == 0 {
				fmt.Println("No crew members found. Create one with: gt crew add <name> --rig", rigName)
				return nil
			}

			fmt.Println("\nAvailable crew members:")
			for i, m := range members {
				fmt.Printf("  %d. %s\n", i+1, m)
			}
			fmt.Print("\nSelect crew member [1]: ")

			var resp string
			fmt.Scanln(&resp)
			resp = strings.TrimSpace(resp)

			if resp == "" {
				crewMember = members[0]
			} else {
				var idx int
				if _, err := fmt.Sscanf(resp, "%d", &idx); err == nil && idx >= 1 && idx <= len(members) {
					crewMember = members[idx-1]
				} else {
					crewMember = members[0]
				}
			}
		}

		// Build work bead description
		var desc strings.Builder
		desc.WriteString(fmt.Sprintf("Address review feedback on PR #%d\n\n", prNum))
		desc.WriteString("## Feedback to Address\n\n")
		for _, r := range reviews {
			if r.State == "CHANGES_REQUESTED" && r.Body != "" {
				desc.WriteString(fmt.Sprintf("- @%s: %s\n", r.Author, epicTruncateString(r.Body, 100)))
			}
		}
		for _, c := range comments {
			if c.Path != "" {
				desc.WriteString(fmt.Sprintf("- @%s on %s: %s\n", c.Author, c.Path, epicTruncateString(c.Body, 100)))
			}
		}
		desc.WriteString(fmt.Sprintf("\nupstream_pr: %d\n", prNum))
		desc.WriteString("review_type: changes_requested\n")

		// Create bead
		fmt.Printf("\nCreating work bead and slinging to %s...\n", crewMember)

		createArgs := []string{"create",
			"--title", fmt.Sprintf("Address PR #%d feedback", prNum),
			"--description", desc.String(),
			"--type", "task",
			"--json",
		}
		createCmd := exec.Command("bd", createArgs...)
		createCmd.Dir = repoDir

		out, err := createCmd.Output()
		if err != nil {
			return fmt.Errorf("creating work bead: %w", err)
		}

		// Parse bead ID from output
		var created []struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(out, &created); err != nil {
			return fmt.Errorf("parsing bead output: %w", err)
		}

		if len(created) == 0 {
			return fmt.Errorf("no bead created")
		}

		beadID := created[0].ID
		fmt.Printf("  Created: %s\n", beadID)

		// Sling to crew
		target := fmt.Sprintf("%s/crew/%s", rigName, crewMember)
		slingArgs := []string{"sling", beadID, target}
		slingCmd := exec.Command("gt", slingArgs...)
		slingCmd.Stdout = os.Stdout
		slingCmd.Stderr = os.Stderr

		if err := slingCmd.Run(); err != nil {
			return fmt.Errorf("slinging work: %w", err)
		}

	case "3":
		fmt.Println("Exiting.")
	}

	return nil
}

type prReview struct {
	Author string
	State  string
	Body   string
}

type prComment struct {
	Author string
	Body   string
	Path   string
	Line   int
}

func getPRReviews(repoDir string, prNum int) ([]prReview, error) {
	cmd := exec.Command("gh", "pr", "view", fmt.Sprintf("%d", prNum), "--json", "reviews")
	cmd.Dir = repoDir

	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return nil, err
	}

	var result struct {
		Reviews []struct {
			Author struct {
				Login string `json:"login"`
			} `json:"author"`
			State string `json:"state"`
			Body  string `json:"body"`
		} `json:"reviews"`
	}

	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		return nil, err
	}

	var reviews []prReview
	for _, r := range result.Reviews {
		reviews = append(reviews, prReview{
			Author: r.Author.Login,
			State:  r.State,
			Body:   r.Body,
		})
	}

	return reviews, nil
}

func getPRComments(repoDir string, prNum int) ([]prComment, error) {
	cmd := exec.Command("gh", "pr", "view", fmt.Sprintf("%d", prNum), "--json", "comments")
	cmd.Dir = repoDir

	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return nil, err
	}

	var result struct {
		Comments []struct {
			Author struct {
				Login string `json:"login"`
			} `json:"author"`
			Body string `json:"body"`
			Path string `json:"path"`
			Line int    `json:"line"`
		} `json:"comments"`
	}

	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		return nil, err
	}

	var comments []prComment
	for _, c := range result.Comments {
		comments = append(comments, prComment{
			Author: c.Author.Login,
			Body:   c.Body,
			Path:   c.Path,
			Line:   c.Line,
		})
	}

	return comments, nil
}

func epicTruncateString(s string, maxLen int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func epicDetectRigFromPath(path, townRoot string) string {
	// Remove town root prefix
	relPath := strings.TrimPrefix(path, townRoot)
	relPath = strings.TrimPrefix(relPath, "/")

	// First component is the rig name
	parts := strings.Split(relPath, "/")
	if len(parts) > 0 && parts[0] != "" && !strings.HasPrefix(parts[0], ".") {
		return parts[0]
	}
	return ""
}
