package web

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/activity"
	"github.com/steveyegge/gastown/internal/workspace"
)

// LiveConvoyFetcher fetches convoy data from beads.
type LiveConvoyFetcher struct {
	townBeads string
}

// NewLiveConvoyFetcher creates a fetcher for the current workspace.
func NewLiveConvoyFetcher() (*LiveConvoyFetcher, error) {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return nil, fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	return &LiveConvoyFetcher{
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
			row.TrackedIssues[i] = TrackedIssue{
				ID:          t.ID,
				Title:       t.Title,
				Status:      t.Status,
				Assignee:    t.Assignee,
				Description: t.Description,
				Priority:    t.Priority,
				Type:        t.Type,
				CreatedAt:   t.CreatedAt,
				UpdatedAt:   t.UpdatedAt.Format("2006-01-02 15:04"),
			}
		}

		rows = append(rows, row)
	}

	return rows, nil
}

// trackedIssueInfo holds info about an issue being tracked by a convoy.
type trackedIssueInfo struct {
	ID           string
	Title        string
	Status       string
	Assignee     string
	Description  string
	Priority     int
	Type         string
	CreatedAt    string
	LastActivity time.Time
	UpdatedAt    time.Time // Fallback for activity when no assignee
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

	// Build result
	result := make([]trackedIssueInfo, 0, len(issueIDs))
	for _, id := range issueIDs {
		info := trackedIssueInfo{ID: id}

		if d, ok := details[id]; ok {
			info.Title = d.Title
			info.Status = d.Status
			info.Assignee = d.Assignee
			info.Description = d.Description
			info.Priority = d.Priority
			info.Type = d.Type
			info.CreatedAt = d.CreatedAt
			info.UpdatedAt = d.UpdatedAt
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
	ID          string
	Title       string
	Status      string
	Assignee    string
	Description string
	Priority    int
	Type        string
	CreatedAt   string
	UpdatedAt   time.Time
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
		ID          string `json:"id"`
		Title       string `json:"title"`
		Status      string `json:"status"`
		Assignee    string `json:"assignee"`
		Description string `json:"description"`
		Priority    int    `json:"priority"`
		Type        string `json:"issue_type"`
		CreatedAt   string `json:"created_at"`
		UpdatedAt   string `json:"updated_at"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &issues); err != nil {
		return result
	}

	for _, issue := range issues {
		detail := &issueDetail{
			ID:          issue.ID,
			Title:       issue.Title,
			Status:      issue.Status,
			Assignee:    issue.Assignee,
			Description: issue.Description,
			Priority:    issue.Priority,
			Type:        issue.Type,
			CreatedAt:   issue.CreatedAt,
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
	// Repos to query for PRs
	repos := []struct {
		Full  string // Full repo path for gh CLI
		Short string // Short name for display
	}{
		{"michaellady/roxas", "roxas"},
		{"michaellady/gastown", "gastown"},
	}

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
	// #nosec G204 -- gh is a trusted CLI, repo is from hardcoded list
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

		polecats = append(polecats, PolecatRow{
			Name:         polecat,
			Rig:          rig,
			SessionID:    sessionName,
			LastActivity: activity.Calculate(activityTime),
			StatusHint:   statusHint,
		})
	}

	return polecats, nil
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

// FetchTaskQueue fetches standalone tasks (open issues not tracked by any convoy).
func (f *LiveConvoyFetcher) FetchTaskQueue() ([]TaskRow, error) {
	// List all open issues (excluding convoys, agents, merge-requests)
	listArgs := []string{"list", "--status=open", "--json"}
	listCmd := exec.Command("bd", listArgs...)
	listCmd.Dir = f.townBeads

	var stdout bytes.Buffer
	listCmd.Stdout = &stdout

	if err := listCmd.Run(); err != nil {
		return nil, fmt.Errorf("listing issues: %w", err)
	}

	var issues []struct {
		ID       string `json:"id"`
		Title    string `json:"title"`
		Status   string `json:"status"`
		Priority int    `json:"priority"`
		Type     string `json:"issue_type"`
		Assignee string `json:"assignee"`
		Labels   string `json:"labels"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &issues); err != nil {
		return nil, fmt.Errorf("parsing issue list: %w", err)
	}

	// Get set of tracked issue IDs (to exclude from task queue)
	trackedIDs := f.getTrackedIssueIDs()

	// Filter to standalone tasks
	var tasks []TaskRow
	for _, issue := range issues {
		// Skip convoys, agents, merge-requests, and tracked issues
		if issue.Type == "convoy" || issue.Type == "agent" || issue.Type == "merge-request" {
			continue
		}
		if strings.Contains(issue.Labels, "gt:agent") || strings.Contains(issue.Labels, "gt:merge-request") {
			continue
		}
		if trackedIDs[issue.ID] {
			continue
		}

		// Determine color class based on priority
		colorClass := ""
		switch issue.Priority {
		case 0, 1:
			colorClass = "task-high"
		case 2:
			colorClass = "task-medium"
		}

		tasks = append(tasks, TaskRow{
			ID:         issue.ID,
			Title:      issue.Title,
			Status:     issue.Status,
			Priority:   issue.Priority,
			Assignee:   issue.Assignee,
			ColorClass: colorClass,
		})
	}

	return tasks, nil
}

// getTrackedIssueIDs returns a set of issue IDs that are tracked by convoys.
func (f *LiveConvoyFetcher) getTrackedIssueIDs() map[string]bool {
	result := make(map[string]bool)
	dbPath := filepath.Join(f.townBeads, "beads.db")

	// Query all tracked dependencies
	queryCmd := exec.Command("sqlite3", "-json", dbPath,
		`SELECT depends_on_id FROM dependencies WHERE type = 'tracks'`)

	var stdout bytes.Buffer
	queryCmd.Stdout = &stdout
	if err := queryCmd.Run(); err != nil {
		return result
	}

	var deps []struct {
		DependsOnID string `json:"depends_on_id"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &deps); err != nil {
		return result
	}

	for _, dep := range deps {
		issueID := dep.DependsOnID
		// Normalize external refs
		if strings.HasPrefix(issueID, "external:") {
			parts := strings.SplitN(issueID, ":", 3)
			if len(parts) == 3 {
				issueID = parts[2]
			}
		}
		result[issueID] = true
	}

	return result
}
