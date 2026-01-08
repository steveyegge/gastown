package web

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/steveyegge/gastown/internal/activity"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/workspace"
)

// LiveConvoyFetcher fetches convoy data from beads.
type LiveConvoyFetcher struct {
	townRoot  string
	townBeads string
}

// NewLiveConvoyFetcher creates a fetcher for the current workspace.
func NewLiveConvoyFetcher() (*LiveConvoyFetcher, error) {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return nil, fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	return &LiveConvoyFetcher{
		townRoot:  townRoot,
		townBeads: filepath.Join(townRoot, ".beads"),
	}, nil
}


// FetchConvoys fetches all open convoys with their activity data.
func (f *LiveConvoyFetcher) FetchConvoys() ([]ConvoyRow, error) {
	// List all open convoy-type issues
	listArgs := []string{"list", "--type=convoy", "--status=open", "--json"}
	listCmd := exec.Command("bd", listArgs...)
	listCmd.Dir = f.townBeads

	var stdout bytes.Buffer
	listCmd.Stdout = &stdout

	if err := listCmd.Run(); err != nil {
		return nil, fmt.Errorf("listing convoys: %w", err)
	}

	var convoys []struct {
		ID        string `json:"id"`
		Title     string `json:"title"`
		Status    string `json:"status"`
		CreatedAt string `json:"created_at"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &convoys); err != nil {
		return nil, fmt.Errorf("parsing convoy list: %w", err)
	}

	// Build convoy rows with activity data
	rows := make([]ConvoyRow, 0, len(convoys))
	for _, c := range convoys {
		row := ConvoyRow{
			ID:     c.ID,
			Title:  c.Title,
			Status: c.Status,
		}

		// Get tracked issues for progress and activity calculation
		tracked := f.getTrackedIssues(c.ID)
		row.Total = len(tracked)

		var mostRecentActivity time.Time
		var mostRecentUpdated time.Time
		var hasAssignee bool
		for _, t := range tracked {
			if t.Status == "closed" {
				row.Completed++
			}
			// Track most recent activity from workers
			if t.LastActivity.After(mostRecentActivity) {
				mostRecentActivity = t.LastActivity
			}
			// Track most recent updated_at as fallback
			if t.UpdatedAt.After(mostRecentUpdated) {
				mostRecentUpdated = t.UpdatedAt
			}
			if t.Assignee != "" {
				hasAssignee = true
			}
		}

		row.Progress = fmt.Sprintf("%d/%d", row.Completed, row.Total)

		// Calculate activity info from most recent worker activity
		if !mostRecentActivity.IsZero() {
			// Have active tmux session activity from assigned workers
			row.LastActivity = activity.Calculate(mostRecentActivity)
		} else if !hasAssignee {
			// No assignees found in beads - try fallback to any running polecat activity
			// This handles cases where bd update --assignee didn't persist or wasn't returned
			if polecatActivity := f.getAllPolecatActivity(); polecatActivity != nil {
				info := activity.Calculate(*polecatActivity)
				info.FormattedAge = info.FormattedAge + " (polecat active)"
				row.LastActivity = info
			} else if !mostRecentUpdated.IsZero() {
				// Fall back to issue updated_at if no polecats running
				info := activity.Calculate(mostRecentUpdated)
				info.FormattedAge = info.FormattedAge + " (unassigned)"
				row.LastActivity = info
			} else {
				row.LastActivity = activity.Info{
					FormattedAge: "unassigned",
					ColorClass:   activity.ColorUnknown,
				}
			}
		} else {
			// Has assignee but no active session
			row.LastActivity = activity.Info{
				FormattedAge: "idle",
				ColorClass:   activity.ColorUnknown,
			}
		}

		// Calculate work status based on progress and activity
		row.WorkStatus = calculateWorkStatus(row.Completed, row.Total, row.LastActivity.ColorClass)

		// Get tracked issues for expandable view
		row.TrackedIssues = make([]TrackedIssue, len(tracked))
		for i, t := range tracked {
			ti := TrackedIssue{
				ID:             t.ID,
				Title:          t.Title,
				Status:         t.Status,
				Assignee:       t.Assignee,
				MoleculeID:     t.MoleculeID,
				CurrentStep:    t.CurrentStep,
				TotalSteps:     t.TotalSteps,
				CompletedSteps: t.CompletedSteps,
			}
			// Format step progress string if molecule exists
			if t.TotalSteps > 0 {
				ti.StepProgress = fmt.Sprintf("%d/%d", t.CompletedSteps, t.TotalSteps)
			}
			row.TrackedIssues[i] = ti
		}

		rows = append(rows, row)
	}

	return rows, nil
}

// trackedIssueInfo holds info about an issue being tracked by a convoy.
type trackedIssueInfo struct {
	ID             string
	Title          string
	Status         string
	Assignee       string
	LastActivity   time.Time
	UpdatedAt      time.Time // Fallback for activity when no assignee
	MoleculeID     string    // Attached molecule ID (if any)
	CurrentStep    string    // Title of current step (in_progress or first ready)
	TotalSteps     int       // Total steps in the molecule
	CompletedSteps int       // Completed steps in the molecule
}

// getTrackedIssues fetches tracked issues for a convoy.
func (f *LiveConvoyFetcher) getTrackedIssues(convoyID string) []trackedIssueInfo {
	dbPath := filepath.Join(f.townBeads, "beads.db")

	// Query tracked dependencies from SQLite
	safeConvoyID := strings.ReplaceAll(convoyID, "'", "''")
	// #nosec G204 -- sqlite3 path is from trusted config, convoyID is escaped
	queryCmd := exec.Command("sqlite3", "-json", dbPath,
		fmt.Sprintf(`SELECT depends_on_id, type FROM dependencies WHERE issue_id = '%s' AND type = 'tracks'`, safeConvoyID))

	var stdout bytes.Buffer
	queryCmd.Stdout = &stdout
	if err := queryCmd.Run(); err != nil {
		return nil
	}

	var deps []struct {
		DependsOnID string `json:"depends_on_id"`
		Type        string `json:"type"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &deps); err != nil {
		return nil
	}

	// Collect issue IDs (normalize external refs)
	issueIDs := make([]string, 0, len(deps))
	for _, dep := range deps {
		issueID := dep.DependsOnID
		if strings.HasPrefix(issueID, "external:") {
			parts := strings.SplitN(issueID, ":", 3)
			if len(parts) == 3 {
				issueID = parts[2]
			}
		}
		issueIDs = append(issueIDs, issueID)
	}

	// Batch fetch issue details
	details := f.getIssueDetailsBatch(issueIDs)

	// Get worker activity from tmux sessions based on assignees
	workers := f.getWorkersFromAssignees(details)

	// Get molecule status for each unique assignee
	moleculeStatus := make(map[string]*moleculeStatusJSON)
	for _, detail := range details {
		if detail != nil && detail.Assignee != "" {
			if _, exists := moleculeStatus[detail.Assignee]; !exists {
				moleculeStatus[detail.Assignee] = f.getMoleculeStatusForAssignee(detail.Assignee)
			}
		}
	}

	// Build result
	result := make([]trackedIssueInfo, 0, len(issueIDs))
	for _, id := range issueIDs {
		info := trackedIssueInfo{ID: id}

		if d, ok := details[id]; ok {
			info.Title = d.Title
			info.Status = d.Status
			info.Assignee = d.Assignee
			info.UpdatedAt = d.UpdatedAt

			// Add molecule info if assignee has an attached molecule
			if molStatus, hasMol := moleculeStatus[d.Assignee]; hasMol && molStatus != nil {
				info.MoleculeID = molStatus.AttachedMolecule
				if molStatus.Progress != nil {
					info.TotalSteps = molStatus.Progress.TotalSteps
					info.CompletedSteps = molStatus.Progress.DoneSteps
					info.CurrentStep = f.getMoleculeCurrentStep(molStatus.Progress)
				}
			}
		} else {
			info.Title = "(external)"
			info.Status = "unknown"
		}

		if w, ok := workers[id]; ok && w.LastActivity != nil {
			info.LastActivity = *w.LastActivity
		}

		result = append(result, info)
	}

	return result
}

// issueDetail holds basic issue info.
type issueDetail struct {
	ID        string
	Title     string
	Status    string
	Assignee  string
	UpdatedAt time.Time
}

// getIssueDetailsBatch fetches details for multiple issues.
func (f *LiveConvoyFetcher) getIssueDetailsBatch(issueIDs []string) map[string]*issueDetail {
	result := make(map[string]*issueDetail)
	if len(issueIDs) == 0 {
		return result
	}

	args := append([]string{"show"}, issueIDs...)
	args = append(args, "--json")

	// #nosec G204 -- bd is a trusted internal tool, args are issue IDs
	showCmd := exec.Command("bd", args...)
	var stdout bytes.Buffer
	showCmd.Stdout = &stdout

	if err := showCmd.Run(); err != nil {
		return result
	}

	var issues []struct {
		ID        string `json:"id"`
		Title     string `json:"title"`
		Status    string `json:"status"`
		Assignee  string `json:"assignee"`
		UpdatedAt string `json:"updated_at"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &issues); err != nil {
		return result
	}

	for _, issue := range issues {
		detail := &issueDetail{
			ID:       issue.ID,
			Title:    issue.Title,
			Status:   issue.Status,
			Assignee: issue.Assignee,
		}
		// Parse updated_at timestamp
		if issue.UpdatedAt != "" {
			if t, err := time.Parse(time.RFC3339, issue.UpdatedAt); err == nil {
				detail.UpdatedAt = t
			}
		}
		result[issue.ID] = detail
	}

	return result
}

// workerDetail holds worker info including last activity.
type workerDetail struct {
	Worker       string
	LastActivity *time.Time
}

// getWorkersFromAssignees gets worker activity from tmux sessions based on issue assignees.
// Assignees are in format "rigname/polecats/polecatname" which maps to tmux session "gt-rigname-polecatname".
func (f *LiveConvoyFetcher) getWorkersFromAssignees(details map[string]*issueDetail) map[string]*workerDetail {
	result := make(map[string]*workerDetail)

	// Collect unique assignees and map them to issue IDs
	assigneeToIssues := make(map[string][]string)
	for issueID, detail := range details {
		if detail == nil || detail.Assignee == "" {
			continue
		}
		assigneeToIssues[detail.Assignee] = append(assigneeToIssues[detail.Assignee], issueID)
	}

	if len(assigneeToIssues) == 0 {
		return result
	}

	// For each unique assignee, look up tmux session activity
	for assignee, issueIDs := range assigneeToIssues {
		activity := f.getSessionActivityForAssignee(assignee)
		if activity == nil {
			continue
		}

		// Apply this activity to all issues assigned to this worker
		for _, issueID := range issueIDs {
			result[issueID] = &workerDetail{
				Worker:       assignee,
				LastActivity: activity,
			}
		}
	}

	return result
}

// moleculeStatusJSON represents the JSON output from 'gt mol status --json'.
type moleculeStatusJSON struct {
	AttachedMolecule string `json:"attached_molecule"`
	Progress         *struct {
		TotalSteps int      `json:"total_steps"`
		DoneSteps  int      `json:"done_steps"`
		InProgress int      `json:"in_progress_steps"`
		ReadySteps []string `json:"ready_steps"`
	} `json:"progress"`
}

// getMoleculeStatusForAssignee queries molecule status for an assignee.
// Returns molecule info if the assignee has an attached molecule with progress.
func (f *LiveConvoyFetcher) getMoleculeStatusForAssignee(assignee string) *moleculeStatusJSON {
	if assignee == "" {
		return nil
	}

	// Query gt mol status for the assignee
	// #nosec G204 -- gt is a trusted internal tool, assignee is from beads data
	cmd := exec.Command("gt", "mol", "status", assignee, "--json")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return nil
	}

	var status moleculeStatusJSON
	if err := json.Unmarshal(stdout.Bytes(), &status); err != nil {
		return nil
	}

	// Only return if there's an attached molecule with progress
	if status.AttachedMolecule == "" || status.Progress == nil {
		return nil
	}

	return &status
}

// getMoleculeCurrentStep fetches the title of the current step for a molecule.
// Returns the first in-progress or first ready step title.
func (f *LiveConvoyFetcher) getMoleculeCurrentStep(progress *struct {
	TotalSteps int      `json:"total_steps"`
	DoneSteps  int      `json:"done_steps"`
	InProgress int      `json:"in_progress_steps"`
	ReadySteps []string `json:"ready_steps"`
}) string {
	if progress == nil {
		return ""
	}

	// If there are ready steps, get the title of the first one
	if len(progress.ReadySteps) > 0 {
		stepID := progress.ReadySteps[0]
		// Query bd show for step title
		// #nosec G204 -- bd is a trusted internal tool
		cmd := exec.Command("bd", "--allow-stale", "show", stepID, "--json")
		cmd.Dir = f.townBeads
		var stdout bytes.Buffer
		cmd.Stdout = &stdout
		if err := cmd.Run(); err != nil {
			return stepID // Fall back to just the ID
		}

		var steps []struct {
			Title string `json:"title"`
		}
		if err := json.Unmarshal(stdout.Bytes(), &steps); err != nil || len(steps) == 0 {
			return stepID
		}
		return steps[0].Title
	}

	return ""
}

// getSessionActivityForAssignee looks up tmux session activity for an assignee.
// Assignee format: "rigname/polecats/polecatname" -> session "gt-rigname-polecatname"
func (f *LiveConvoyFetcher) getSessionActivityForAssignee(assignee string) *time.Time {
	// Parse assignee: "roxas/polecats/dag" -> rig="roxas", polecat="dag"
	parts := strings.Split(assignee, "/")
	if len(parts) != 3 || parts[1] != "polecats" {
		return nil
	}
	rig := parts[0]
	polecat := parts[2]

	// Construct session name
	sessionName := fmt.Sprintf("gt-%s-%s", rig, polecat)

	// Query tmux for session activity
	// Format: session_activity returns unix timestamp
	cmd := exec.Command("tmux", "list-sessions", "-F", "#{session_name}|#{session_activity}",
		"-f", fmt.Sprintf("#{==:#{session_name},%s}", sessionName))
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return nil
	}

	output := strings.TrimSpace(stdout.String())
	if output == "" {
		return nil
	}

	// Parse output: "gt-roxas-dag|1704312345"
	outputParts := strings.Split(output, "|")
	if len(outputParts) < 2 {
		return nil
	}

	var activityUnix int64
	if _, err := fmt.Sscanf(outputParts[1], "%d", &activityUnix); err != nil || activityUnix == 0 {
		return nil
	}

	activity := time.Unix(activityUnix, 0)
	return &activity
}

// getAllPolecatActivity returns the most recent activity from any running polecat session.
// This is used as a fallback when no specific assignee activity can be determined.
// Returns nil if no polecat sessions are running.
func (f *LiveConvoyFetcher) getAllPolecatActivity() *time.Time {
	// List all tmux sessions matching gt-*-* pattern (polecat sessions)
	// Format: gt-{rig}-{polecat}
	cmd := exec.Command("tmux", "list-sessions", "-F", "#{session_name}|#{session_activity}")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return nil
	}

	var mostRecent time.Time
	for _, line := range strings.Split(stdout.String(), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.Split(line, "|")
		if len(parts) < 2 {
			continue
		}

		sessionName := parts[0]
		// Check if it's a polecat session (gt-{rig}-{polecat}, not gt-{rig}-witness/refinery)
		// Polecat sessions have exactly 3 parts when split by "-" and the middle part is the rig
		nameParts := strings.Split(sessionName, "-")
		if len(nameParts) < 3 || nameParts[0] != "gt" {
			continue
		}
		// Skip witness, refinery, mayor, deacon sessions
		lastPart := nameParts[len(nameParts)-1]
		if lastPart == "witness" || lastPart == "refinery" || lastPart == "mayor" || lastPart == "deacon" {
			continue
		}

		var activityUnix int64
		if _, err := fmt.Sscanf(parts[1], "%d", &activityUnix); err != nil || activityUnix == 0 {
			continue
		}

		activityTime := time.Unix(activityUnix, 0)
		if activityTime.After(mostRecent) {
			mostRecent = activityTime
		}
	}

	if mostRecent.IsZero() {
		return nil
	}
	return &mostRecent
}

// calculateWorkStatus determines the work status based on progress and activity.
// Returns: "complete", "active", "stale", "stuck", or "waiting"
func calculateWorkStatus(completed, total int, activityColor string) string {
	// Check if all work is done
	if total > 0 && completed == total {
		return "complete"
	}

	// Determine status based on activity color
	switch activityColor {
	case activity.ColorGreen:
		return "active"
	case activity.ColorYellow:
		return "stale"
	case activity.ColorRed:
		return "stuck"
	default:
		return "waiting"
	}
}

// FetchMergeQueue fetches open PRs from configured repos.
func (f *LiveConvoyFetcher) FetchMergeQueue() ([]MergeQueueRow, error) {
	// Discover repos from the rigs registry
	repos := f.discoverGitHubRepos()

	var result []MergeQueueRow

	for _, repo := range repos {
		prs, err := f.fetchPRsForRepo(repo.Full, repo.Short)
		if err != nil {
			// Non-fatal: continue with other repos
			continue
		}
		result = append(result, prs...)
	}

	return result, nil
}

// repoInfo holds GitHub repo information for PR fetching.
type repoInfo struct {
	Full  string // Full repo path for gh CLI (owner/repo)
	Short string // Short name for display (rig name)
}

// discoverGitHubRepos loads the rigs registry and extracts GitHub repos.
func (f *LiveConvoyFetcher) discoverGitHubRepos() []repoInfo {
	rigsPath := filepath.Join(f.townRoot, "mayor", "rigs.json")
	rigsConfig, err := config.LoadRigsConfig(rigsPath)
	if err != nil {
		return nil
	}

	var repos []repoInfo
	for rigName, entry := range rigsConfig.Rigs {
		// Parse GitHub repo from git URL
		ownerRepo := parseGitHubRepo(entry.GitURL)
		if ownerRepo == "" {
			// Not a GitHub URL (e.g., file://, local path)
			continue
		}
		repos = append(repos, repoInfo{
			Full:  ownerRepo,
			Short: rigName,
		})
	}

	return repos
}

// parseGitHubRepo extracts owner/repo from various git URL formats.
// Returns empty string if not a GitHub URL.
// Supported formats:
//   - git@github.com:owner/repo.git
//   - https://github.com/owner/repo.git
//   - https://github.com/owner/repo
//   - ssh://git@github.com/owner/repo.git
func parseGitHubRepo(gitURL string) string {
	// Skip non-GitHub URLs
	if strings.HasPrefix(gitURL, "file://") || !strings.Contains(gitURL, "github.com") {
		return ""
	}

	// SSH format: git@github.com:owner/repo.git
	sshPattern := regexp.MustCompile(`git@github\.com:([^/]+)/([^/]+?)(?:\.git)?$`)
	if matches := sshPattern.FindStringSubmatch(gitURL); len(matches) == 3 {
		return matches[1] + "/" + matches[2]
	}

	// HTTPS format: https://github.com/owner/repo.git or https://github.com/owner/repo
	httpsPattern := regexp.MustCompile(`https://github\.com/([^/]+)/([^/]+?)(?:\.git)?$`)
	if matches := httpsPattern.FindStringSubmatch(gitURL); len(matches) == 3 {
		return matches[1] + "/" + matches[2]
	}

	// SSH with ssh:// prefix: ssh://git@github.com/owner/repo.git
	sshPrefixPattern := regexp.MustCompile(`ssh://git@github\.com/([^/]+)/([^/]+?)(?:\.git)?$`)
	if matches := sshPrefixPattern.FindStringSubmatch(gitURL); len(matches) == 3 {
		return matches[1] + "/" + matches[2]
	}

	return ""
}

// prResponse represents the JSON response from gh pr list.
type prResponse struct {
	Number            int    `json:"number"`
	Title             string `json:"title"`
	URL               string `json:"url"`
	Mergeable         string `json:"mergeable"`
	StatusCheckRollup []struct {
		State      string `json:"state"`
		Status     string `json:"status"`
		Conclusion string `json:"conclusion"`
	} `json:"statusCheckRollup"`
}

// fetchPRsForRepo fetches open PRs for a single repo.
func (f *LiveConvoyFetcher) fetchPRsForRepo(repoFull, repoShort string) ([]MergeQueueRow, error) {
	// #nosec G204 -- gh is a trusted CLI, repo is from rigs registry
	cmd := exec.Command("gh", "pr", "list",
		"--repo", repoFull,
		"--state", "open",
		"--json", "number,title,url,mergeable,statusCheckRollup")

	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("fetching PRs for %s: %w", repoFull, err)
	}

	var prs []prResponse
	if err := json.Unmarshal(stdout.Bytes(), &prs); err != nil {
		return nil, fmt.Errorf("parsing PRs for %s: %w", repoFull, err)
	}

	result := make([]MergeQueueRow, 0, len(prs))
	for _, pr := range prs {
		row := MergeQueueRow{
			Number: pr.Number,
			Repo:   repoShort,
			Title:  pr.Title,
			URL:    pr.URL,
		}

		// Determine CI status from statusCheckRollup
		row.CIStatus = determineCIStatus(pr.StatusCheckRollup)

		// Determine mergeable status
		row.Mergeable = determineMergeableStatus(pr.Mergeable)

		// Determine color class based on overall status
		row.ColorClass = determineColorClass(row.CIStatus, row.Mergeable)

		result = append(result, row)
	}

	return result, nil
}

// determineCIStatus evaluates the overall CI status from status checks.
func determineCIStatus(checks []struct {
	State      string `json:"state"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
}) string {
	if len(checks) == 0 {
		return "pending"
	}

	hasFailure := false
	hasPending := false

	for _, check := range checks {
		// Check conclusion first (for completed checks)
		switch check.Conclusion {
		case "failure", "cancelled", "timed_out", "action_required": //nolint:misspell // GitHub API returns "cancelled" (British spelling)
			hasFailure = true
		case "success", "skipped", "neutral":
			// Pass
		default:
			// Check status for in-progress checks
			switch check.Status {
			case "queued", "in_progress", "waiting", "pending", "requested":
				hasPending = true
			}
			// Also check state field
			switch check.State {
			case "FAILURE", "ERROR":
				hasFailure = true
			case "PENDING", "EXPECTED":
				hasPending = true
			}
		}
	}

	if hasFailure {
		return "fail"
	}
	if hasPending {
		return "pending"
	}
	return "pass"
}

// determineMergeableStatus converts GitHub's mergeable field to display value.
func determineMergeableStatus(mergeable string) string {
	switch strings.ToUpper(mergeable) {
	case "MERGEABLE":
		return "ready"
	case "CONFLICTING":
		return "conflict"
	default:
		return "pending"
	}
}

// determineColorClass determines the row color based on CI and merge status.
func determineColorClass(ciStatus, mergeable string) string {
	if ciStatus == "fail" || mergeable == "conflict" {
		return "mq-red"
	}
	if ciStatus == "pending" || mergeable == "pending" {
		return "mq-yellow"
	}
	if ciStatus == "pass" && mergeable == "ready" {
		return "mq-green"
	}
	return "mq-yellow"
}

// FetchPolecats fetches all running polecat and refinery sessions with activity data.
func (f *LiveConvoyFetcher) FetchPolecats() ([]PolecatRow, error) {
	// Query all tmux sessions with window_activity for more accurate timing
	cmd := exec.Command("tmux", "list-sessions", "-F", "#{session_name}|#{window_activity}")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		// tmux not running or no sessions
		return nil, nil
	}

	// Pre-fetch merge queue count to determine refinery idle status
	mergeQueueCount := f.getMergeQueueCount()

	// Pre-fetch session costs for all sessions
	sessionCosts := f.getSessionCosts()

	var polecats []PolecatRow
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")

	for _, line := range lines {
		if line == "" {
			continue
		}

		parts := strings.Split(line, "|")
		if len(parts) < 2 {
			continue
		}

		sessionName := parts[0]

		// Filter for gt-<rig>-<polecat> pattern
		if !strings.HasPrefix(sessionName, "gt-") {
			continue
		}

		// Parse session name: gt-roxas-dag -> rig=roxas, polecat=dag
		nameParts := strings.SplitN(sessionName, "-", 3)
		if len(nameParts) != 3 {
			continue
		}
		rig := nameParts[1]
		polecat := nameParts[2]

		// Skip non-worker sessions (witness, mayor, deacon, boot)
		// Note: refinery is included to show idle/processing status
		if polecat == "witness" || polecat == "mayor" || polecat == "deacon" || polecat == "boot" {
			continue
		}

		// Parse activity timestamp
		var activityUnix int64
		if _, err := fmt.Sscanf(parts[1], "%d", &activityUnix); err != nil || activityUnix == 0 {
			continue
		}
		activityTime := time.Unix(activityUnix, 0)

		// Get status hint - special handling for refinery
		var statusHint string
		if polecat == "refinery" {
			statusHint = f.getRefineryStatusHint(mergeQueueCount)
		} else {
			statusHint = f.getPolecatStatusHint(sessionName)
		}

		// Look up session cost
		cost := sessionCosts[sessionName]

		polecats = append(polecats, PolecatRow{
			Name:         polecat,
			Rig:          rig,
			SessionID:    sessionName,
			LastActivity: activity.Calculate(activityTime),
			StatusHint:   statusHint,
			SessionCost:  cost,
		})
	}

	return polecats, nil
}

// getSessionCosts fetches costs for all sessions from 'gt costs --json'.
// Returns a map of session name to cost in USD.
func (f *LiveConvoyFetcher) getSessionCosts() map[string]float64 {
	result := make(map[string]float64)

	// #nosec G204 -- gt is a trusted internal tool
	cmd := exec.Command("gt", "costs", "--json")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return result
	}

	var costsResp struct {
		Sessions []struct {
			Session string  `json:"session"`
			CostUSD float64 `json:"cost_usd"`
		} `json:"sessions"`
	}

	if err := json.Unmarshal(stdout.Bytes(), &costsResp); err != nil {
		return result
	}

	for _, s := range costsResp.Sessions {
		result[s.Session] = s.CostUSD
	}

	return result
}

// getPolecatStatusHint captures the last non-empty line from a polecat's pane.
func (f *LiveConvoyFetcher) getPolecatStatusHint(sessionName string) string {
	cmd := exec.Command("tmux", "capture-pane", "-t", sessionName, "-p", "-J")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return ""
	}

	// Get last non-empty line
	lines := strings.Split(stdout.String(), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line != "" {
			// Truncate long lines
			if len(line) > 60 {
				line = line[:57] + "..."
			}
			return line
		}
	}
	return ""
}

// getMergeQueueCount returns the total number of open PRs across all repos.
func (f *LiveConvoyFetcher) getMergeQueueCount() int {
	mergeQueue, err := f.FetchMergeQueue()
	if err != nil {
		return 0
	}
	return len(mergeQueue)
}

// getRefineryStatusHint returns appropriate status for refinery based on merge queue.
func (f *LiveConvoyFetcher) getRefineryStatusHint(mergeQueueCount int) string {
	if mergeQueueCount == 0 {
		return "Idle - Waiting for PRs"
	}
	if mergeQueueCount == 1 {
		return "Processing 1 PR"
	}
	return fmt.Sprintf("Processing %d PRs", mergeQueueCount)
}

// truncateStatusHint truncates a status hint to 60 characters with ellipsis.
func truncateStatusHint(line string) string {
	if len(line) > 60 {
		return line[:57] + "..."
	}
	return line
}

// parsePolecatSessionName parses a tmux session name into rig and polecat components.
// Format: gt-<rig>-<polecat> -> (rig, polecat, true)
// Returns ("", "", false) if the format is invalid.
func parsePolecatSessionName(sessionName string) (rig, polecat string, ok bool) {
	if !strings.HasPrefix(sessionName, "gt-") {
		return "", "", false
	}
	parts := strings.SplitN(sessionName, "-", 3)
	if len(parts) != 3 {
		return "", "", false
	}
	return parts[1], parts[2], true
}

// isWorkerSession returns true if the polecat name represents a worker session.
// Non-worker sessions: witness, mayor, deacon, boot
func isWorkerSession(polecat string) bool {
	switch polecat {
	case "witness", "mayor", "deacon", "boot":
		return false
	default:
		return true
	}
}

// parseActivityTimestamp parses a Unix timestamp string from tmux.
// Returns (0, false) for invalid or zero timestamps.
func parseActivityTimestamp(s string) (int64, bool) {
	var unix int64
	if _, err := fmt.Sscanf(s, "%d", &unix); err != nil || unix <= 0 {
		return 0, false
	}
	return unix, true
}

// FetchPolecatDetail fetches detailed information about a specific polecat.
func (f *LiveConvoyFetcher) FetchPolecatDetail(rig, name string) (*PolecatDetailData, error) {
	sessionName := fmt.Sprintf("gt-%s-%s", rig, name)

	// Get session activity
	cmd := exec.Command("tmux", "list-sessions", "-F", "#{session_name}|#{window_activity}",
		"-f", fmt.Sprintf("#{==:#{session_name},%s}", sessionName))
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("session not found: %s", sessionName)
	}

	output := strings.TrimSpace(stdout.String())
	if output == "" {
		return nil, fmt.Errorf("session not found: %s", sessionName)
	}

	// Parse activity timestamp
	parts := strings.Split(output, "|")
	var activityInfo activity.Info
	if len(parts) >= 2 {
		var activityUnix int64
		if _, err := fmt.Sscanf(parts[1], "%d", &activityUnix); err == nil && activityUnix > 0 {
			activityTime := time.Unix(activityUnix, 0)
			activityInfo = activity.Calculate(activityTime)
		}
	}

	// Get pane output (last 20 lines)
	paneOutput := f.getPolecatPaneOutput(sessionName, 20)

	// Get hooked work
	hookedWork := f.getPolecatHookedWork(rig, name)

	return &PolecatDetailData{
		Name:         name,
		Rig:          rig,
		SessionID:    sessionName,
		LastActivity: activityInfo,
		HookedWork:   hookedWork,
		PaneOutput:   paneOutput,
	}, nil
}

// getPolecatPaneOutput captures the last n lines from a polecat's pane.
func (f *LiveConvoyFetcher) getPolecatPaneOutput(sessionName string, lines int) string {
	cmd := exec.Command("tmux", "capture-pane", "-t", sessionName, "-p", "-J")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return ""
	}

	allLines := strings.Split(stdout.String(), "\n")

	// Filter out empty lines from the end and get last n non-empty lines
	var nonEmptyLines []string
	for i := len(allLines) - 1; i >= 0 && len(nonEmptyLines) < lines; i-- {
		line := strings.TrimRight(allLines[i], " \t\r")
		if line != "" {
			nonEmptyLines = append([]string{line}, nonEmptyLines...)
		}
	}

	return strings.Join(nonEmptyLines, "\n")
}

// getPolecatHookedWork returns the hooked work ID for a polecat if any.
func (f *LiveConvoyFetcher) getPolecatHookedWork(rig, name string) string {
	assignee := fmt.Sprintf("%s/polecats/%s", rig, name)

	// Query bd for hooked issues assigned to this polecat
	cmd := exec.Command("bd", "list", "--status=hooked", "--json")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return ""
	}

	var issues []struct {
		ID       string `json:"id"`
		Assignee string `json:"assignee"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &issues); err != nil {
		return ""
	}

	for _, issue := range issues {
		if issue.Assignee == assignee {
			return issue.ID
		}
	}
	return ""
}

// FeedEvent represents an event for the SSE feed.
type FeedEvent struct {
	ID        string    `json:"id"`
	Time      time.Time `json:"time"`
	Type      string    `json:"type"`
	Actor     string    `json:"actor"`
	Target    string    `json:"target,omitempty"`
	Message   string    `json:"message"`
	Rig       string    `json:"rig,omitempty"`
	Raw       string    `json:"raw,omitempty"`
}

// ActivityWatcher combines events from .events.jsonl and bd activity.
type ActivityWatcher struct {
	townRoot string
	events   chan FeedEvent
	cancel   context.CancelFunc
	wg       sync.WaitGroup

	// Deduplication: track recent event keys
	mu         sync.Mutex
	recentKeys map[string]time.Time
}

// NewActivityWatcher creates a new activity watcher.
func NewActivityWatcher(townRoot string) (*ActivityWatcher, error) {
	if townRoot == "" {
		var err error
		townRoot, err = workspace.FindFromCwdOrError()
		if err != nil {
			return nil, fmt.Errorf("not in a Gas Town workspace: %w", err)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())

	w := &ActivityWatcher{
		townRoot:   townRoot,
		events:     make(chan FeedEvent, 100),
		cancel:     cancel,
		recentKeys: make(map[string]time.Time),
	}

	// Start tailing .events.jsonl
	w.wg.Add(1)
	go w.tailEventsFile(ctx)

	// Start tailing bd activity
	w.wg.Add(1)
	go w.tailBdActivity(ctx)

	// Start deduplication cleanup goroutine
	w.wg.Add(1)
	go w.cleanupDedup(ctx)

	return w, nil
}

// Events returns the event channel.
func (w *ActivityWatcher) Events() <-chan FeedEvent {
	return w.events
}

// Close stops the watcher.
func (w *ActivityWatcher) Close() error {
	w.cancel()
	w.wg.Wait()
	close(w.events)
	return nil
}

// tailEventsFile tails ~/gt/.events.jsonl for new events.
func (w *ActivityWatcher) tailEventsFile(ctx context.Context) {
	defer w.wg.Done()

	eventsPath := filepath.Join(w.townRoot, ".events.jsonl")
	file, err := os.Open(eventsPath)
	if err != nil {
		// File might not exist yet - that's ok
		return
	}
	defer file.Close()

	// Seek to end for live tailing
	_, _ = file.Seek(0, 2)

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	scanner := bufio.NewScanner(file)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for scanner.Scan() {
				line := scanner.Text()
				if event := w.parseGtEventLine(line); event != nil {
					if w.dedupEvent(event) {
						select {
						case w.events <- *event:
						default:
							// Drop if channel full
						}
					}
				}
			}
		}
	}
}

// GtEventJSON is the structure of events in .events.jsonl.
type GtEventJSON struct {
	Timestamp  string                 `json:"ts"`
	Source     string                 `json:"source"`
	Type       string                 `json:"type"`
	Actor      string                 `json:"actor"`
	Payload    map[string]interface{} `json:"payload"`
	Visibility string                 `json:"visibility"`
}

// parseGtEventLine parses a line from .events.jsonl.
func (w *ActivityWatcher) parseGtEventLine(line string) *FeedEvent {
	if strings.TrimSpace(line) == "" {
		return nil
	}

	var ge GtEventJSON
	if err := json.Unmarshal([]byte(line), &ge); err != nil {
		return nil
	}

	// Only show feed-visible events
	if ge.Visibility != "feed" && ge.Visibility != "both" {
		return nil
	}

	t, err := time.Parse(time.RFC3339, ge.Timestamp)
	if err != nil {
		t = time.Now()
	}

	// Extract rig from payload or actor
	rig := ""
	if ge.Payload != nil {
		if r, ok := ge.Payload["rig"].(string); ok {
			rig = r
		}
	}
	if rig == "" && ge.Actor != "" {
		parts := strings.Split(ge.Actor, "/")
		if len(parts) > 0 && parts[0] != "mayor" && parts[0] != "deacon" {
			rig = parts[0]
		}
	}

	// Build message from event type and payload
	message := w.buildEventMessage(ge.Type, ge.Payload)

	target := ""
	if ge.Payload != nil {
		if b, ok := ge.Payload["bead"].(string); ok {
			target = b
		}
	}

	return &FeedEvent{
		ID:      fmt.Sprintf("gt-%s-%d", ge.Type, t.UnixNano()),
		Time:    t,
		Type:    ge.Type,
		Actor:   ge.Actor,
		Target:  target,
		Message: message,
		Rig:     rig,
		Raw:     line,
	}
}

// buildEventMessage creates a human-readable message from event type and payload.
func (w *ActivityWatcher) buildEventMessage(eventType string, payload map[string]interface{}) string {
	getString := func(key string) string {
		if payload == nil {
			return ""
		}
		if v, ok := payload[key].(string); ok {
			return v
		}
		return ""
	}

	switch eventType {
	case "sling":
		bead := getString("bead")
		target := getString("target")
		if bead != "" && target != "" {
			return fmt.Sprintf("slung %s to %s", bead, target)
		}
		return "work slung"

	case "hook":
		bead := getString("bead")
		if bead != "" {
			return fmt.Sprintf("hooked %s", bead)
		}
		return "bead hooked"

	case "handoff":
		subject := getString("subject")
		if subject != "" {
			return fmt.Sprintf("handoff: %s", subject)
		}
		return "session handoff"

	case "done":
		bead := getString("bead")
		if bead != "" {
			return fmt.Sprintf("done: %s", bead)
		}
		return "work done"

	case "mail":
		subject := getString("subject")
		to := getString("to")
		if subject != "" {
			if to != "" {
				return fmt.Sprintf("→ %s: %s", to, subject)
			}
			return subject
		}
		return "mail sent"

	case "patrol_started", "patrol_complete":
		if msg := getString("message"); msg != "" {
			return msg
		}
		return eventType

	case "polecat_nudged":
		polecat := getString("polecat")
		reason := getString("reason")
		if polecat != "" {
			if reason != "" {
				return fmt.Sprintf("nudged %s: %s", polecat, reason)
			}
			return fmt.Sprintf("nudged %s", polecat)
		}
		return "polecat nudged"

	case "merged":
		worker := getString("worker")
		if worker != "" {
			return fmt.Sprintf("merged work from %s", worker)
		}
		return "merged"

	case "merge_failed":
		reason := getString("reason")
		if reason != "" {
			return fmt.Sprintf("merge failed: %s", reason)
		}
		return "merge failed"

	default:
		if msg := getString("message"); msg != "" {
			return msg
		}
		return eventType
	}
}

// tailBdActivity tails bd activity --follow for beads events.
func (w *ActivityWatcher) tailBdActivity(ctx context.Context) {
	defer w.wg.Done()

	// Find beads directory
	beadsDir := filepath.Join(w.townRoot, ".beads")
	if _, err := os.Stat(beadsDir); os.IsNotExist(err) {
		// Try rig-level beads
		// For now, just use town-level
		return
	}

	cmd := exec.CommandContext(ctx, "bd", "activity", "--follow")
	cmd.Dir = w.townRoot

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return
	}

	if err := cmd.Start(); err != nil {
		return
	}

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return
		default:
		}

		line := scanner.Text()
		if event := w.parseBdActivityLine(line); event != nil {
			if w.dedupEvent(event) {
				select {
				case w.events <- *event:
				default:
					// Drop if channel full
				}
			}
		}
	}

	_ = cmd.Wait()
}

// bd activity line pattern: [HH:MM:SS] SYMBOL BEAD_ID action · description
var bdActivityPattern = regexp.MustCompile(`^\[(\d{2}:\d{2}:\d{2})\]\s+([+→✓✗⊘📌])\s+(\S+)?\s*(\w+)?\s*·?\s*(.*)$`)

// parseBdActivityLine parses a line from bd activity output.
func (w *ActivityWatcher) parseBdActivityLine(line string) *FeedEvent {
	matches := bdActivityPattern.FindStringSubmatch(line)
	if matches == nil {
		return w.parseSimpleBdLine(line)
	}

	timeStr := matches[1]
	symbol := matches[2]
	beadID := matches[3]
	action := matches[4]
	message := matches[5]

	// Parse time (assume today)
	now := time.Now()
	t, err := time.Parse("15:04:05", timeStr)
	if err != nil {
		t = now
	} else {
		t = time.Date(now.Year(), now.Month(), now.Day(), t.Hour(), t.Minute(), t.Second(), 0, now.Location())
	}

	// Map symbol to event type
	eventType := "update"
	switch symbol {
	case "+":
		eventType = "create"
	case "→":
		eventType = "update"
	case "✓":
		eventType = "complete"
	case "✗":
		eventType = "fail"
	case "⊘":
		eventType = "delete"
	case "📌":
		eventType = "pin"
	}

	return &FeedEvent{
		ID:      fmt.Sprintf("bd-%s-%d", beadID, t.UnixNano()),
		Time:    t,
		Type:    eventType,
		Target:  beadID,
		Message: strings.TrimSpace(action + " " + message),
		Raw:     line,
	}
}

// parseSimpleBdLine handles lines that don't match the full pattern.
func (w *ActivityWatcher) parseSimpleBdLine(line string) *FeedEvent {
	if strings.TrimSpace(line) == "" {
		return nil
	}

	// Try to extract timestamp
	var t time.Time
	if len(line) > 10 && line[0] == '[' {
		if idx := strings.Index(line, "]"); idx > 0 {
			timeStr := line[1:idx]
			now := time.Now()
			if parsed, err := time.Parse("15:04:05", timeStr); err == nil {
				t = time.Date(now.Year(), now.Month(), now.Day(),
					parsed.Hour(), parsed.Minute(), parsed.Second(), 0, now.Location())
			}
		}
	}

	if t.IsZero() {
		t = time.Now()
	}

	return &FeedEvent{
		ID:      fmt.Sprintf("bd-raw-%d", t.UnixNano()),
		Time:    t,
		Type:    "update",
		Message: line,
		Raw:     line,
	}
}

// dedupEvent checks if an event is a duplicate. Returns true if event should be sent.
func (w *ActivityWatcher) dedupEvent(event *FeedEvent) bool {
	// Create a dedup key based on type, target, and message
	key := fmt.Sprintf("%s:%s:%s", event.Type, event.Target, event.Message)

	w.mu.Lock()
	defer w.mu.Unlock()

	if lastSeen, ok := w.recentKeys[key]; ok {
		// Duplicate if seen within 2 seconds
		if time.Since(lastSeen) < 2*time.Second {
			return false
		}
	}

	w.recentKeys[key] = time.Now()
	return true
}

// cleanupDedup periodically cleans up old dedup entries.
func (w *ActivityWatcher) cleanupDedup(ctx context.Context) {
	defer w.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.mu.Lock()
			cutoff := time.Now().Add(-30 * time.Second)
			for key, ts := range w.recentKeys {
				if ts.Before(cutoff) {
					delete(w.recentKeys, key)
				}
			}
			w.mu.Unlock()
		}
	}
}

// WatchActivity is a convenience function that creates a watcher and returns its event channel.
func WatchActivity() (<-chan FeedEvent, func(), error) {
	watcher, err := NewActivityWatcher("")
	if err != nil {
		return nil, nil, err
	}

	cleanup := func() {
		_ = watcher.Close()
	}

	return watcher.Events(), cleanup, nil
}
