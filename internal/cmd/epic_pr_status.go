package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/beads"
	epicpkg "github.com/steveyegge/gastown/internal/epic"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

var epicPRStatusJSON bool

var epicPRStatusCmd = &cobra.Command{
	Use:   "status [epic-id]",
	Short: "Show upstream PR status for epic",
	Long: `Show status of all upstream PRs created for an epic.

Displays:
- PR number and title
- Review status (approved, changes requested, pending)
- CI status
- Target branch
- Number of approvals

EXAMPLES:

  gt epic pr status gt-epic-abc12
  gt epic pr status                   # Uses hooked epic
  gt epic pr status gt-epic-abc12 --json`,
	Args: cobra.MaximumNArgs(1),
	RunE: runEpicPRStatus,
}

func init() {
	epicPRStatusCmd.Flags().BoolVar(&epicPRStatusJSON, "json", false, "Output as JSON")
}

func runEpicPRStatus(cmd *cobra.Command, args []string) error {
	// Find town root
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return err
	}

	// Determine epic ID
	var epicID string
	if len(args) > 0 {
		epicID = args[0]
	} else {
		epicID, err = getHookedEpicID()
		if err != nil {
			return fmt.Errorf("no epic specified and no epic hooked: %w", err)
		}
	}

	// Get rig from epic ID
	rigName, err := getRigFromBeadID(epicID)
	if err != nil {
		return fmt.Errorf("could not determine rig: %w", err)
	}

	rigPath := filepath.Join(townRoot, rigName)
	beadsDir := filepath.Join(rigPath, "mayor", "rig")
	repoDir := filepath.Join(rigPath, "mayor", "rig")

	// Get epic bead
	bd := beads.New(beadsDir)
	epic, fields, err := bd.GetEpicBead(epicID)
	if err != nil {
		return fmt.Errorf("getting epic: %w", err)
	}
	if epic == nil {
		return fmt.Errorf("epic %s not found", epicID)
	}

	// Parse PR URLs
	prURLs := epicpkg.ParseUpstreamPRs(fields.UpstreamPRs)
	if len(prURLs) == 0 {
		fmt.Println("No upstream PRs found for this epic.")
		fmt.Println("Submit PRs with: gt epic submit", epicID)
		return nil
	}

	// Get status for each PR
	type prStatus struct {
		Number     int    `json:"number"`
		URL        string `json:"url"`
		Title      string `json:"title"`
		State      string `json:"state"`
		Base       string `json:"base"`
		ReviewStat string `json:"review_status"`
		Approvals  int    `json:"approvals"`
		CIStatus   string `json:"ci_status"`
		CIDetails  string `json:"ci_details,omitempty"`
	}

	var statuses []prStatus
	var actions []string

	for _, url := range prURLs {
		_, _, prNum, err := epicpkg.ParsePRURL(url)
		if err != nil {
			continue
		}

		// Get PR details using gh
		prInfo, err := getPRDetails(repoDir, prNum)
		if err != nil {
			statuses = append(statuses, prStatus{
				Number: prNum,
				URL:    url,
				State:  "unknown",
			})
			continue
		}

		// Get review status
		reviewStatus, approvals, _ := epicpkg.GetPRReviewStatus(repoDir, prNum)

		// Get CI status
		ciStatus, _ := epicpkg.GetPRCIStatus(repoDir, prNum)
		ciState := "unknown"
		ciDetails := ""
		if ciStatus != nil {
			ciState = ciStatus.State
			ciDetails = ciStatus.Details
		}

		status := prStatus{
			Number:     prNum,
			URL:        url,
			Title:      prInfo.Title,
			State:      prInfo.State,
			Base:       prInfo.Base,
			ReviewStat: reviewStatus,
			Approvals:  approvals,
			CIStatus:   ciState,
			CIDetails:  ciDetails,
		}
		statuses = append(statuses, status)

		// Determine actions needed
		if reviewStatus == epicpkg.PRStatusChangesRequested {
			actions = append(actions, fmt.Sprintf("PR #%d: Address requested changes", prNum))
		}
		if ciState == "failure" {
			actions = append(actions, fmt.Sprintf("PR #%d: Fix CI failures", prNum))
		}
	}

	if epicPRStatusJSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(statuses)
	}

	// Human-readable output
	fmt.Printf("%s %s\n\n", style.Bold.Render("Epic:"), epic.Title)
	fmt.Printf("%s\n\n", style.Bold.Render("Upstream PRs:"))

	for _, st := range statuses {
		// Status icon
		icon := "○"
		switch st.ReviewStat {
		case epicpkg.PRStatusApproved:
			icon = "✓"
		case epicpkg.PRStatusChangesRequested:
			icon = "✗"
		case "pending", "review_required":
			icon = "⏳"
		}

		// State
		stateStr := st.State
		if st.State == "MERGED" {
			stateStr = style.Success.Render("merged")
		} else if st.State == "CLOSED" {
			stateStr = style.Dim.Render("closed")
		}

		// Approvals
		approvalStr := ""
		if st.Approvals > 0 {
			approvalStr = fmt.Sprintf("%d approval(s)", st.Approvals)
		}

		// CI
		ciStr := ""
		switch st.CIStatus {
		case "success":
			ciStr = style.Success.Render("CI ✓")
		case "failure":
			ciStr = style.Error.Render("CI ✗")
		case "pending":
			ciStr = style.Warning.Render("CI ⏳")
		}

		fmt.Printf("  %s #%d %-30s %s → %s\n", icon, st.Number, epicTruncate(st.Title, 30), stateStr, st.Base)
		if approvalStr != "" || ciStr != "" {
			fmt.Printf("      %s %s\n", approvalStr, ciStr)
		}
	}

	if len(actions) > 0 {
		fmt.Printf("\n%s\n", style.Bold.Render("Actions needed:"))
		for _, action := range actions {
			fmt.Printf("  - %s\n", action)
		}
	}

	return nil
}

type prDetails struct {
	Title string
	State string
	Base  string
}

func getPRDetails(repoDir string, prNum int) (*prDetails, error) {
	cmd := exec.Command("gh", "pr", "view", fmt.Sprintf("%d", prNum), "--json", "title,state,baseRefName")
	cmd.Dir = repoDir

	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return nil, err
	}

	var result struct {
		Title       string `json:"title"`
		State       string `json:"state"`
		BaseRefName string `json:"baseRefName"`
	}

	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		return nil, err
	}

	return &prDetails{
		Title: result.Title,
		State: result.State,
		Base:  result.BaseRefName,
	}, nil
}

func epicTruncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
