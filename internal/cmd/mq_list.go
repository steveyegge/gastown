package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/refinery"
	"github.com/steveyegge/gastown/internal/style"
)

func runMQList(cmd *cobra.Command, args []string) error {
	rigName := args[0]

	_, r, _, err := getRefineryManager(rigName)
	if err != nil {
		return err
	}

	// Create beads wrapper for the rig - use BeadsPath() to get the git-synced location
	b := beads.New(r.BeadsPath())

	// Build list options - query for gt:merge-request label
	// Priority -1 means no priority filter (otherwise 0 would filter to P0 only)
	// Use Label filter instead of Type (bd-3q6.10: MRs now use type=task)
	opts := beads.ListOptions{
		Label:    "gt:merge-request",
		Priority: -1,
	}

	// Apply status filter if specified
	if mqListStatus != "" {
		opts.Status = mqListStatus
	} else if !mqListReady {
		// Default to open if not showing ready
		opts.Status = "open"
	}

	var issues []*beads.Issue

	if mqListReady {
		// Use ready query which filters by no blockers
		allReady, err := b.Ready()
		if err != nil {
			return fmt.Errorf("querying ready MRs: %w", err)
		}
		// Filter to only merge-request label (issue_type field is deprecated)
		for _, issue := range allReady {
			if beads.HasLabel(issue, "gt:merge-request") {
				issues = append(issues, issue)
			}
		}
	} else {
		issues, err = b.List(opts)
		if err != nil {
			return fmt.Errorf("querying merge queue: %w", err)
		}
	}

	// Apply additional filters and calculate scores
	now := time.Now()
	type scoredIssue struct {
		issue  *beads.Issue
		fields *beads.MRFields
		score  float64
	}
	var scored []scoredIssue

	for _, issue := range issues {
		// Manual status filtering as workaround for bd list not respecting --status filter
		if mqListReady {
			// Ready view should only show open MRs
			if issue.Status != "open" {
				continue
			}
		} else if mqListStatus != "" && !strings.EqualFold(mqListStatus, "all") {
			// Explicit status filter should match exactly
			if !strings.EqualFold(issue.Status, mqListStatus) {
				continue
			}
		} else if mqListStatus == "" && issue.Status != "open" {
			// Default case (no status specified) should only show open
			continue
		}

		// Parse MR fields
		fields := beads.ParseMRFields(issue)

		// Filter by worker
		if mqListWorker != "" {
			worker := ""
			if fields != nil {
				worker = fields.Worker
			}
			if !strings.EqualFold(worker, mqListWorker) {
				continue
			}
		}

		// Filter by epic (target branch)
		if mqListEpic != "" {
			target := ""
			if fields != nil {
				target = fields.Target
			}
			expectedTarget := "integration/" + mqListEpic
			if target != expectedTarget {
				continue
			}
		}

		// Filter by rig - only show MRs for this rig
		// MRs have a rig field indicating which rig they belong to.
		// Skip MRs that have a rig field that doesn't match the requested rig.
		if fields != nil && fields.Rig != "" && !strings.EqualFold(fields.Rig, rigName) {
			continue
		}

		// Calculate priority score
		score := calculateMRScore(issue, fields, now)
		scored = append(scored, scoredIssue{issue: issue, fields: fields, score: score})
	}

	// Sort by score descending (highest priority first)
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	// Extract filtered issues for JSON output compatibility
	var filtered []*beads.Issue
	for _, s := range scored {
		filtered = append(filtered, s.issue)
	}

	// JSON output
	if mqListJSON {
		return outputJSON(filtered)
	}

	// Human-readable output
	fmt.Printf("%s Merge queue for '%s':\n\n", style.Bold.Render("📋"), rigName)

	if len(filtered) == 0 {
		fmt.Printf("  %s\n", style.Dim.Render("(empty)"))
		return nil
	}

	// Create styled table with SCORE column
	table := style.NewTable(
		style.Column{Name: "ID", Width: 12},
		style.Column{Name: "SCORE", Width: 7, Align: style.AlignRight},
		style.Column{Name: "PRI", Width: 4},
		style.Column{Name: "CONVOY", Width: 12},
		style.Column{Name: "BRANCH", Width: 24},
		style.Column{Name: "STATUS", Width: 10},
		style.Column{Name: "AGE", Width: 6, Align: style.AlignRight},
	)

	// Add rows using scored items (already sorted by score)
	for _, item := range scored {
		issue := item.issue
		fields := item.fields

		// Determine display status
		displayStatus := issue.Status
		if issue.Status == "open" {
			if len(issue.BlockedBy) > 0 || issue.BlockedByCount > 0 {
				displayStatus = "blocked"
			} else {
				displayStatus = "ready"
			}
		}

		// Format status with styling
		styledStatus := displayStatus
		switch displayStatus {
		case "ready":
			styledStatus = style.Success.Render("ready")
		case "in_progress":
			styledStatus = style.Warning.Render("active")
		case "blocked":
			styledStatus = style.Dim.Render("blocked")
		case "closed":
			styledStatus = style.Dim.Render("closed")
		}

		// Get MR fields
		branch := ""
		convoyID := ""
		if fields != nil {
			branch = fields.Branch
			convoyID = fields.ConvoyID
		}

		// Format convoy column
		convoyDisplay := style.Dim.Render("(none)")
		if convoyID != "" {
			// Truncate convoy ID for display
			if len(convoyID) > 12 {
				convoyID = convoyID[:12]
			}
			convoyDisplay = convoyID
		}

		// Format priority with color
		priority := fmt.Sprintf("P%d", issue.Priority)
		if issue.Priority <= 1 {
			priority = style.Error.Render(priority)
		} else if issue.Priority == 2 {
			priority = style.Warning.Render(priority)
		}

		// Format score
		scoreStr := fmt.Sprintf("%.1f", item.score)

		// Calculate age
		age := formatMRAge(issue.CreatedAt)

		// Truncate ID if needed
		displayID := issue.ID
		if len(displayID) > 12 {
			displayID = displayID[:12]
		}

		table.AddRow(displayID, scoreStr, priority, convoyDisplay, branch, styledStatus, style.Dim.Render(age))
	}

	fmt.Print(table.Render())

	// Show blocking details below table
	for _, item := range scored {
		issue := item.issue
		displayStatus := issue.Status
		if issue.Status == "open" && (len(issue.BlockedBy) > 0 || issue.BlockedByCount > 0) {
			displayStatus = "blocked"
		}
		if displayStatus == "blocked" && len(issue.BlockedBy) > 0 {
			displayID := issue.ID
			if len(displayID) > 12 {
				displayID = displayID[:12]
			}
			fmt.Printf("  %s %s\n", style.Dim.Render(displayID+":"),
				style.Dim.Render(fmt.Sprintf("waiting on %s", issue.BlockedBy[0])))
		}
	}

	return nil
}

// formatMRAge formats the age of an MR from its created_at timestamp.
func formatMRAge(createdAt string) string {
	t, err := time.Parse(time.RFC3339, createdAt)
	if err != nil {
		// Try other formats
		t, err = time.Parse("2006-01-02T15:04:05Z", createdAt)
		if err != nil {
			return "?"
		}
	}

	d := time.Since(t)

	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

// outputJSON outputs data as JSON.
func outputJSON(data interface{}) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(data)
}

// calculateMRScore computes the priority score for an MR using the refinery scoring function.
// Higher scores mean higher priority (process first).
func calculateMRScore(issue *beads.Issue, fields *beads.MRFields, now time.Time) float64 {
	// Parse MR creation time
	mrCreatedAt, err := time.Parse(time.RFC3339, issue.CreatedAt)
	if err != nil {
		mrCreatedAt, err = time.Parse("2006-01-02T15:04:05Z", issue.CreatedAt)
		if err != nil {
			mrCreatedAt = now // Fallback to now if parsing fails
		}
	}

	// Build score input
	input := refinery.ScoreInput{
		Priority:    issue.Priority,
		MRCreatedAt: mrCreatedAt,
		Now:         now,
	}

	// Add fields from MR metadata if available
	if fields != nil {
		input.RetryCount = fields.RetryCount

		// Parse convoy created at if available
		if fields.ConvoyCreatedAt != "" {
			if convoyTime, err := time.Parse(time.RFC3339, fields.ConvoyCreatedAt); err == nil {
				input.ConvoyCreatedAt = &convoyTime
			}
		}
	}

	return refinery.ScoreMRWithDefaults(input)
}
