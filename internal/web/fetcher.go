package web

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
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
			// No workers assigned to any tracked issues
			if !mostRecentUpdated.IsZero() {
				// Show when issue was last updated
				info := activity.Calculate(mostRecentUpdated)
				info.FormattedAge = info.FormattedAge + " (no worker)"
				row.LastActivity = info
			} else {
				row.LastActivity = activity.Info{
					FormattedAge: "no worker",
					ColorClass:   activity.ColorUnknown,
				}
			}
		} else {
			// Has assignee but no active tmux session (worker crashed or stopped)
			row.LastActivity = activity.Info{
				FormattedAge: "worker offline",
				ColorClass:   activity.ColorRed,
			}
		}

		// Calculate work status based on progress and activity
		row.WorkStatus = calculateWorkStatus(row.Completed, row.Total, row.LastActivity.ColorClass)

		// Get tracked issues for expandable view
		row.TrackedIssues = make([]TrackedIssue, len(tracked))
		for i, t := range tracked {
			row.TrackedIssues[i] = TrackedIssue{
				ID:       t.ID,
				Title:    t.Title,
				Status:   t.Status,
				Assignee: t.Assignee,
			}
		}

		rows = append(rows, row)
	}

	// Sort convoys: stuck first, then stale, then active, then waiting, then complete
	// Within same status, sort by ID for consistency
	statusOrder := map[string]int{"stuck": 0, "stale": 1, "active": 2, "waiting": 3, "complete": 4}
	sort.Slice(rows, func(i, j int) bool {
		oi, oj := statusOrder[rows[i].WorkStatus], statusOrder[rows[j].WorkStatus]
		if oi != oj {
			return oi < oj
		}
		return rows[i].ID < rows[j].ID
	})

	return rows, nil
}

// trackedIssueInfo holds info about an issue being tracked by a convoy.
type trackedIssueInfo struct {
	ID           string
	Title        string
	Status       string
	Assignee     string
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

	// Query tmux for window activity (more accurate than session_activity).
	// session_activity only updates on I/O, but window_activity tracks cursor
	// movement and other changes - important when Claude is "thinking" for long periods.
	cmd := exec.Command("tmux", "list-sessions", "-F", "#{session_name}|#{window_activity}",
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
	// Use window_activity instead of session_activity for better accuracy during long "thinking" periods.
	cmd := exec.Command("tmux", "list-sessions", "-F", "#{session_name}|#{window_activity}")
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

// FetchMergeQueue fetches open PRs from registered rigs.
func (f *LiveConvoyFetcher) FetchMergeQueue() ([]MergeQueueRow, error) {
	// Load registered rigs from config
	rigsConfigPath := filepath.Join(f.townRoot, "mayor", "rigs.json")
	rigsConfig, err := config.LoadRigsConfig(rigsConfigPath)
	if err != nil {
		return nil, fmt.Errorf("loading rigs config: %w", err)
	}

	var result []MergeQueueRow

	for rigName, entry := range rigsConfig.Rigs {
		// Convert git URL to owner/repo format for gh CLI
		repoPath := gitURLToRepoPath(entry.GitURL)
		if repoPath == "" {
			continue
		}

		prs, err := f.fetchPRsForRepo(repoPath, rigName)
		if err != nil {
			// Non-fatal: continue with other repos
			continue
		}
		result = append(result, prs...)
	}

	// Sort merge queue: ready PRs first, then by repo and PR number
	colorOrder := map[string]int{"mq-green": 0, "mq-yellow": 1, "mq-red": 2}
	sort.Slice(result, func(i, j int) bool {
		oi, oj := colorOrder[result[i].ColorClass], colorOrder[result[j].ColorClass]
		if oi != oj {
			return oi < oj
		}
		if result[i].Repo != result[j].Repo {
			return result[i].Repo < result[j].Repo
		}
		return result[i].Number < result[j].Number
	})

	return result, nil
}

// gitURLToRepoPath converts a git URL to owner/repo format.
// Supports HTTPS (https://github.com/owner/repo.git) and
// SSH (git@github.com:owner/repo.git) formats.
func gitURLToRepoPath(gitURL string) string {
	// Handle HTTPS format: https://github.com/owner/repo.git
	if strings.HasPrefix(gitURL, "https://github.com/") {
		path := strings.TrimPrefix(gitURL, "https://github.com/")
		path = strings.TrimSuffix(path, ".git")
		return path
	}

	// Handle SSH format: git@github.com:owner/repo.git
	if strings.HasPrefix(gitURL, "git@github.com:") {
		path := strings.TrimPrefix(gitURL, "git@github.com:")
		path = strings.TrimSuffix(path, ".git")
		return path
	}

	// Unsupported format
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
	// #nosec G204 -- gh is a trusted CLI, repo is from registered rigs config
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
	// Load registered rigs to filter sessions
	rigsConfigPath := filepath.Join(f.townRoot, "mayor", "rigs.json")
	rigsConfig, err := config.LoadRigsConfig(rigsConfigPath)
	if err != nil {
		return nil, fmt.Errorf("loading rigs config: %w", err)
	}

	// Build list of registered rig names (sorted by length descending for longest-match-first)
	rigNames := make([]string, 0, len(rigsConfig.Rigs))
	for rigName := range rigsConfig.Rigs {
		rigNames = append(rigNames, rigName)
	}
	// Sort by length descending so "my-rig" matches before "my"
	sort.Slice(rigNames, func(i, j int) bool {
		return len(rigNames[i]) > len(rigNames[j])
	})

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

		// Parse session name by matching against registered rig names.
		// This handles rig names with hyphens (e.g., "gt-my-rig-chrome" where rig is "my-rig").
		suffix := strings.TrimPrefix(sessionName, "gt-")
		var rig, polecat string
		for _, rigName := range rigNames {
			prefix := rigName + "-"
			if strings.HasPrefix(suffix, prefix) {
				rig = rigName
				polecat = strings.TrimPrefix(suffix, prefix)
				break
			}
		}
		if rig == "" || polecat == "" {
			continue
		}

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
			statusHint = f.getStatusHintWithDefault(sessionName)
		}

		polecats = append(polecats, PolecatRow{
			Name:         polecat,
			Rig:          rig,
			SessionID:    sessionName,
			LastActivity: activity.Calculate(activityTime),
			StatusHint:   statusHint,
		})
	}

	// Sort polecats by rig, then by name
	sort.Slice(polecats, func(i, j int) bool {
		if polecats[i].Rig != polecats[j].Rig {
			return polecats[i].Rig < polecats[j].Rig
		}
		return polecats[i].Name < polecats[j].Name
	})

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

	// Get last non-empty line, skipping Claude UI elements
	lines := strings.Split(stdout.String(), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line != "" && !isClaudeUILine(line) {
			// Truncate long lines
			if len(line) > 60 {
				line = line[:57] + "..."
			}
			return line
		}
	}
	return ""
}

// isClaudeUILine returns true if the line is a Claude Code UI element (not actual output).
func isClaudeUILine(line string) bool {
	// Skip Claude status/permission lines (start with play symbols)
	if strings.HasPrefix(line, "⏵") {
		return true
	}
	// Skip lines that are just the prompt indicator
	if strings.HasPrefix(line, ">") && len(line) < 5 {
		return true
	}
	// Skip the send button indicator
	if strings.Contains(line, "↵") && strings.Contains(line, "send") {
		return true
	}
	// Skip lines that are mostly box-drawing characters (UI separators)
	if isBoxDrawingLine(line) {
		return true
	}
	// Skip common Claude UI patterns
	uiPatterns := []string{
		"bypass permissions",
		"shift+tab to cycle",
		"to cycle)",
		"Esc to interrupt",
		"to interrupt",
		"? for shortcuts",
		"for shortcuts",
		"claude-code",
		"Claude Code",
	}
	for _, pattern := range uiPatterns {
		if strings.Contains(line, pattern) {
			return true
		}
	}
	return false
}

// isBoxDrawingLine returns true if a line is mostly box-drawing characters (UI separators).
func isBoxDrawingLine(line string) bool {
	if len(line) < 5 {
		return false
	}
	boxChars := 0
	totalChars := 0
	for _, r := range line {
		totalChars++
		// Box drawing characters: ─ │ ┌ ┐ └ ┘ ├ ┤ ┬ ┴ ┼ ═ ║ etc.
		if r >= 0x2500 && r <= 0x257F {
			boxChars++
		}
	}
	// If more than 50% are box drawing chars, it's a UI line
	return totalChars > 0 && float64(boxChars)/float64(totalChars) > 0.5
}

// isIdleLine returns true if the line indicates Claude is idle/waiting for input.
func isIdleLine(line string) bool {
	idlePatterns := []string{
		"Waiting for",
		"waiting for",
		"Press Enter",
		"Type a message",
		"What would you like",
		"How can I help",
		"Ready to help",
		"What can I",
		"I can help",
	}
	for _, pattern := range idlePatterns {
		if strings.Contains(line, pattern) {
			return true
		}
	}
	// Check if it's just a prompt character
	if line == ">" || line == "$" || line == "%" {
		return true
	}
	return false
}

// getStatusHintWithDefault returns a status hint, or a default if none found.
func (f *LiveConvoyFetcher) getStatusHintWithDefault(sessionName string) string {
	hint := f.getPolecatStatusHint(sessionName)
	if hint == "" {
		return "<idle>"
	}
	if isIdleLine(hint) {
		return "<idle>"
	}
	return hint
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

// FetchPolecatDetail returns expanded information for a polecat session.
func (f *LiveConvoyFetcher) FetchPolecatDetail(sessionID string) (*PolecatDetail, error) {
	// Parse session ID to get rig and polecat name
	// Format: gt-<rig>-<polecat>
	if !strings.HasPrefix(sessionID, "gt-") {
		return nil, fmt.Errorf("invalid session ID format: %s", sessionID)
	}

	// Load rigs to properly parse session name
	rigsConfigPath := filepath.Join(f.townRoot, "mayor", "rigs.json")
	rigsConfig, err := config.LoadRigsConfig(rigsConfigPath)
	if err != nil {
		return nil, fmt.Errorf("loading rigs config: %w", err)
	}

	// Build rig names sorted by length for matching
	rigNames := make([]string, 0, len(rigsConfig.Rigs))
	for rigName := range rigsConfig.Rigs {
		rigNames = append(rigNames, rigName)
	}
	sort.Slice(rigNames, func(i, j int) bool {
		return len(rigNames[i]) > len(rigNames[j])
	})

	// Parse rig and polecat from session name
	suffix := strings.TrimPrefix(sessionID, "gt-")
	var rig, polecatName string
	for _, rigName := range rigNames {
		prefix := rigName + "-"
		if strings.HasPrefix(suffix, prefix) {
			rig = rigName
			polecatName = strings.TrimPrefix(suffix, prefix)
			break
		}
	}
	if rig == "" || polecatName == "" {
		return nil, fmt.Errorf("could not parse session ID: %s", sessionID)
	}

	// Get session activity
	cmd := exec.Command("tmux", "list-sessions", "-F", "#{session_name}|#{window_activity}|#{session_created}",
		"-f", fmt.Sprintf("#{==:#{session_name},%s}", sessionID))
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}

	output := strings.TrimSpace(stdout.String())
	if output == "" {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}

	parts := strings.Split(output, "|")
	if len(parts) < 3 {
		return nil, fmt.Errorf("unexpected tmux output format")
	}

	// Parse activity and created timestamps
	var activityUnix, createdUnix int64
	fmt.Sscanf(parts[1], "%d", &activityUnix)
	fmt.Sscanf(parts[2], "%d", &createdUnix)

	activityTime := time.Unix(activityUnix, 0)
	createdTime := time.Unix(createdUnix, 0)
	uptime := formatDuration(time.Since(createdTime))

	// Get terminal lines
	terminalLines := f.getPolecatTerminalLines(sessionID, 15)

	// Get hook bead from agent bead
	hookBead, hookTitle := f.getPolecatHook(rig, polecatName)

	// Get status hint
	statusHint := f.getStatusHintWithDefault(sessionID)

	return &PolecatDetail{
		PolecatRow: PolecatRow{
			Name:         polecatName,
			Rig:          rig,
			SessionID:    sessionID,
			LastActivity: activity.Calculate(activityTime),
			StatusHint:   statusHint,
		},
		HookBead:      hookBead,
		HookTitle:     hookTitle,
		Uptime:        uptime,
		TerminalLines: terminalLines,
	}, nil
}

// getPolecatTerminalLines returns the last N lines from a polecat's tmux pane.
func (f *LiveConvoyFetcher) getPolecatTerminalLines(sessionID string, n int) []string {
	cmd := exec.Command("tmux", "capture-pane", "-t", sessionID, "-p", "-J", "-S", fmt.Sprintf("-%d", n))
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return nil
	}

	allLines := strings.Split(stdout.String(), "\n")
	// Filter empty lines and trim
	var result []string
	for _, line := range allLines {
		trimmed := strings.TrimRight(line, " \t")
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}

	// Return last N non-empty lines
	if len(result) > n {
		result = result[len(result)-n:]
	}
	return result
}

// getPolecatHook returns the hook bead and title for a polecat.
func (f *LiveConvoyFetcher) getPolecatHook(rig, polecatName string) (string, string) {
	// Query agent bead for hook
	agentBeadID := fmt.Sprintf("gt-%s-polecat-%s", rig, polecatName)
	cmd := exec.Command("bd", "show", agentBeadID, "--json")
	cmd.Dir = f.townBeads
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return "", ""
	}

	var beads []struct {
		HookBead string `json:"hook_bead"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &beads); err != nil || len(beads) == 0 {
		return "", ""
	}

	hookBead := beads[0].HookBead
	if hookBead == "" {
		return "", ""
	}

	// Get hook title
	hookCmd := exec.Command("bd", "show", hookBead, "--json")
	hookCmd.Dir = f.townBeads
	var hookStdout bytes.Buffer
	hookCmd.Stdout = &hookStdout
	if err := hookCmd.Run(); err != nil {
		return hookBead, ""
	}

	var hookBeads []struct {
		Title string `json:"title"`
	}
	if err := json.Unmarshal(hookStdout.Bytes(), &hookBeads); err != nil || len(hookBeads) == 0 {
		return hookBead, ""
	}

	return hookBead, hookBeads[0].Title
}

// formatDuration formats a duration as a human-readable string.
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	hours := int(d.Hours())
	mins := int(d.Minutes()) % 60
	if mins == 0 {
		return fmt.Sprintf("%dh", hours)
	}
	return fmt.Sprintf("%dh %dm", hours, mins)
}

// FetchConvoyDetail returns expanded information for a convoy.
func (f *LiveConvoyFetcher) FetchConvoyDetail(convoyID string) (*ConvoyDetail, error) {
	// Get convoy basic info
	cmd := exec.Command("bd", "show", convoyID, "--json")
	cmd.Dir = f.townBeads
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("convoy not found: %s", convoyID)
	}

	var convoys []struct {
		ID          string `json:"id"`
		Title       string `json:"title"`
		Status      string `json:"status"`
		CreatedAt   string `json:"created_at"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &convoys); err != nil || len(convoys) == 0 {
		return nil, fmt.Errorf("could not parse convoy: %s", convoyID)
	}

	convoy := convoys[0]

	// Get tracked issues with details
	tracked := f.getTrackedIssues(convoyID)
	details := f.getIssueDetailsBatch(collectIssueIDs(tracked))
	workers := f.getWorkersFromAssignees(details)

	var trackedDetails []TrackedIssueDetail
	completed := 0
	for _, t := range tracked {
		detail := TrackedIssueDetail{
			TrackedIssue: TrackedIssue{
				ID:       t.ID,
				Title:    t.Title,
				Status:   t.Status,
				Assignee: t.Assignee,
			},
		}

		if t.Status == "closed" {
			completed++
		}

		// Parse assignee to get short name
		if t.Assignee != "" {
			parts := strings.Split(t.Assignee, "/")
			if len(parts) > 0 {
				detail.AssigneeShort = parts[len(parts)-1]
			}
		}

		// Get worker activity
		if w, ok := workers[t.ID]; ok && w.LastActivity != nil {
			detail.AssigneeActivity = activity.Calculate(*w.LastActivity)
		}

		// Get last commit (from git log in polecat worktree)
		if t.Assignee != "" {
			commit, ago := f.getLastCommitForIssue(t.Assignee, t.ID)
			detail.LastCommit = commit
			detail.LastCommitAgo = ago
		}

		trackedDetails = append(trackedDetails, detail)
	}

	// Calculate work status
	var mostRecentActivity time.Time
	for _, t := range tracked {
		if t.LastActivity.After(mostRecentActivity) {
			mostRecentActivity = t.LastActivity
		}
	}

	var lastActivity activity.Info
	if !mostRecentActivity.IsZero() {
		lastActivity = activity.Calculate(mostRecentActivity)
	} else {
		lastActivity = activity.Info{FormattedAge: "unknown", ColorClass: activity.ColorUnknown}
	}

	workStatus := calculateWorkStatus(completed, len(tracked), lastActivity.ColorClass)

	// Parse created time
	createdAgo := ""
	if convoy.CreatedAt != "" {
		if t, err := time.Parse(time.RFC3339, convoy.CreatedAt); err == nil {
			createdAgo = formatDuration(time.Since(t)) + " ago"
		}
	}

	return &ConvoyDetail{
		ConvoyRow: ConvoyRow{
			ID:           convoy.ID,
			Title:        convoy.Title,
			Status:       convoy.Status,
			WorkStatus:   workStatus,
			Progress:     fmt.Sprintf("%d/%d", completed, len(tracked)),
			Completed:    completed,
			Total:        len(tracked),
			LastActivity: lastActivity,
		},
		TrackedDetails: trackedDetails,
		CreatedAt:      convoy.CreatedAt,
		CreatedAgo:     createdAgo,
		Description:    convoy.Description,
	}, nil
}

// collectIssueIDs extracts issue IDs from tracked issues.
func collectIssueIDs(tracked []trackedIssueInfo) []string {
	ids := make([]string, len(tracked))
	for i, t := range tracked {
		ids[i] = t.ID
	}
	return ids
}

// getLastCommitForIssue gets the last commit message for an issue from the assignee's worktree.
func (f *LiveConvoyFetcher) getLastCommitForIssue(assignee, issueID string) (string, string) {
	// Parse assignee: "rigname/polecats/polecatname"
	parts := strings.Split(assignee, "/")
	if len(parts) != 3 || parts[1] != "polecats" {
		return "", ""
	}
	rigName := parts[0]
	polecatName := parts[2]

	// Construct polecat worktree path: townRoot/rigName/polecats/polecatName/rigName
	worktreePath := filepath.Join(f.townRoot, rigName, "polecats", polecatName, rigName)

	// Get last commit
	cmd := exec.Command("git", "log", "-1", "--format=%s|%ar")
	cmd.Dir = worktreePath
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return "", ""
	}

	output := strings.TrimSpace(stdout.String())
	commitParts := strings.SplitN(output, "|", 2)
	if len(commitParts) < 2 {
		return output, ""
	}

	// Truncate long commit messages
	msg := commitParts[0]
	if len(msg) > 50 {
		msg = msg[:47] + "..."
	}

	return msg, commitParts[1]
}

// FetchMergeHistory returns recent commits pushed to main branch via refinery.
func (f *LiveConvoyFetcher) FetchMergeHistory(limit int) ([]MergeHistoryRow, error) {
	rigsConfigPath := filepath.Join(f.townRoot, "mayor", "rigs.json")
	rigsConfig, err := config.LoadRigsConfig(rigsConfigPath)
	if err != nil {
		return nil, fmt.Errorf("loading rigs config: %w", err)
	}

	var result []MergeHistoryRow

	for rigName, entry := range rigsConfig.Rigs {
		// Check refinery clone path: townRoot/rigName/refinery/rig
		refineryPath := filepath.Join(f.townRoot, rigName, "refinery", "rig")

		// Fetch latest from origin
		fetchCmd := exec.Command("git", "fetch", "origin")
		fetchCmd.Dir = refineryPath
		_ = fetchCmd.Run() // Ignore errors, proceed with potentially stale data

		// Get recent commits on origin/main with stats
		cmd := exec.Command("git", "log", "origin/main", "--format=%H|%s|%ar|%aI", "--numstat", "-n", fmt.Sprintf("%d", limit))
		cmd.Dir = refineryPath
		var stdout bytes.Buffer
		cmd.Stdout = &stdout
		if err := cmd.Run(); err != nil {
			continue
		}

		// Build GitHub commit URL base if possible
		commitURLBase := ""
		repoPath := gitURLToRepoPath(entry.GitURL)
		if repoPath != "" {
			commitURLBase = fmt.Sprintf("https://github.com/%s/commit/", repoPath)
		}

		// Parse output - format is header line, blank line, then numstat lines
		// Git outputs: header\n\nnumstat1\nnumstat2\n\nheader2\n\nnumstat3...
		lines := strings.Split(stdout.String(), "\n")
		var currentRow *MergeHistoryRow
		for _, line := range lines {
			if line == "" {
				// Skip blank lines - they appear between header and numstat
				continue
			}

			// Check if this is a commit header line (contains |)
			if strings.Contains(line, "|") && !strings.HasPrefix(line, "-") {
				parts := strings.SplitN(line, "|", 4)
				if len(parts) >= 4 {
					// Save previous commit if exists
					if currentRow != nil {
						result = append(result, *currentRow)
					}

					hash := parts[0]
					shortHash := hash[:8]
					msg := parts[1]

					// Skip uninteresting merge commits
					if strings.HasPrefix(msg, "Merge remote-tracking branch") ||
						strings.HasPrefix(msg, "Merge branch") {
						continue
					}

					if len(msg) > 50 {
						msg = msg[:47] + "..."
					}

					commitURL := ""
					if commitURLBase != "" {
						commitURL = commitURLBase + hash
					}

					currentRow = &MergeHistoryRow{
						Number:    0,
						Repo:      rigName,
						Title:     fmt.Sprintf("[%s] %s", shortHash, msg),
						URL:       commitURL,
						MergedAt:  parts[3],
						MergedAgo: parts[2],
						Success:   true,
						Hash:      shortHash,
					}
				}
			} else if currentRow != nil {
				// This is a numstat line: additions\tdeletions\tfilename
				statParts := strings.Split(line, "\t")
				if len(statParts) >= 3 {
					var add, del int
					fmt.Sscanf(statParts[0], "%d", &add)
					fmt.Sscanf(statParts[1], "%d", &del)
					currentRow.Additions += add
					currentRow.Deletions += del

					// Track top files (limit to 5)
					if len(currentRow.FilesChanged) < 5 {
						filename := statParts[2]
						// Shorten long paths
						if len(filename) > 40 {
							filename = "..." + filename[len(filename)-37:]
						}
						currentRow.FilesChanged = append(currentRow.FilesChanged, fmt.Sprintf("%s (+%d/-%d)", filename, add, del))
					}
				}
			}
		}
		// Don't forget the last commit
		if currentRow != nil {
			result = append(result, *currentRow)
		}
	}

	// Sort by commit time (most recent first)
	sort.Slice(result, func(i, j int) bool {
		return result[i].MergedAt > result[j].MergedAt
	})

	// Limit results
	if len(result) > limit {
		result = result[:limit]
	}

	return result, nil
}

// FetchActivity returns recent activity events from various sources.
func (f *LiveConvoyFetcher) FetchActivity(limit int) ([]ActivityEvent, error) {
	var events []ActivityEvent

	// Add lifecycle events from .events.jsonl (spawn, nudge, handoff, etc.)
	lifecycleEvents := f.fetchLifecycleEvents(limit * 2) // Get more to filter
	events = append(events, lifecycleEvents...)

	// Add git commits from polecats
	commitEvents := f.fetchCommitEvents(limit)
	events = append(events, commitEvents...)

	// Add main branch commits (direct pushes) from refinery clones
	mainCommits := f.fetchMainBranchCommits(limit)
	events = append(events, mainCommits...)

	// Sort by timestamp descending
	sort.Slice(events, func(i, j int) bool {
		return events[i].Timestamp > events[j].Timestamp
	})

	// Limit results
	if len(events) > limit {
		events = events[:limit]
	}

	return events, nil
}

// eventLogEntry matches the structure in .events.jsonl
type eventLogEntry struct {
	Timestamp  string                 `json:"ts"`
	Source     string                 `json:"source"`
	Type       string                 `json:"type"`
	Actor      string                 `json:"actor"`
	Payload    map[string]interface{} `json:"payload,omitempty"`
	Visibility string                 `json:"visibility"`
}

// fetchLifecycleEvents reads events from .events.jsonl file.
func (f *LiveConvoyFetcher) fetchLifecycleEvents(limit int) []ActivityEvent {
	var events []ActivityEvent

	eventsPath := filepath.Join(f.townRoot, ".events.jsonl")
	file, err := os.Open(eventsPath) //nolint:gosec // G304: path is constructed from trusted townRoot
	if err != nil {
		return events
	}
	defer file.Close()

	// Read all lines to get the most recent ones
	var allEntries []eventLogEntry
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var entry eventLogEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		allEntries = append(allEntries, entry)
	}

	// Get last N entries (most recent)
	startIdx := 0
	if len(allEntries) > limit {
		startIdx = len(allEntries) - limit
	}
	recentEntries := allEntries[startIdx:]

	// Convert to ActivityEvents, filtering for interesting types
	for _, entry := range recentEntries {
		event := f.convertEventLogEntry(entry)
		if event != nil {
			events = append(events, *event)
		}
	}

	return events
}

// convertEventLogEntry converts a raw event log entry to an ActivityEvent.
// Returns nil for events we don't want to display in the feed.
func (f *LiveConvoyFetcher) convertEventLogEntry(entry eventLogEntry) *ActivityEvent {
	// Parse timestamp
	ts, err := time.Parse(time.RFC3339, entry.Timestamp)
	if err != nil {
		return nil
	}
	timestampAgo := formatTimeAgo(ts)

	// Extract common payload fields
	rig, _ := entry.Payload["rig"].(string)
	polecat, _ := entry.Payload["polecat"].(string)
	target, _ := entry.Payload["target"].(string)
	reason, _ := entry.Payload["reason"].(string)

	// Determine actor type
	actorType := "system"
	if strings.Contains(entry.Actor, "/polecat") || polecat != "" {
		actorType = "polecat"
	} else if strings.Contains(entry.Actor, "witness") {
		actorType = "witness"
	} else if strings.Contains(entry.Actor, "mayor") {
		actorType = "mayor"
	} else if strings.Contains(entry.Actor, "deacon") {
		actorType = "deacon"
	} else if strings.Contains(entry.Actor, "refinery") {
		actorType = "refinery"
	}

	// Short actor name for display
	actorName := entry.Actor
	if parts := strings.Split(entry.Actor, "/"); len(parts) > 0 {
		actorName = parts[len(parts)-1]
	}

	switch entry.Type {
	case "spawn":
		name := polecat
		if name == "" {
			name = actorName
		}
		return &ActivityEvent{
			ID:           fmt.Sprintf("spawn-%s-%s", entry.Timestamp, name),
			Timestamp:    entry.Timestamp,
			TimestampAgo: timestampAgo,
			Type:         "spawn",
			Actor:        name,
			ActorType:    actorType,
			Summary:      fmt.Sprintf("🐾 Spawned on %s", rig),
			Rig:          rig,
		}

	case "nudge":
		targetName := target
		if targetName == "" {
			targetName = polecat
		}
		reasonText := reason
		if reasonText == "" {
			reasonText = "keepalive"
		}
		return &ActivityEvent{
			ID:           fmt.Sprintf("nudge-%s-%s", entry.Timestamp, targetName),
			Timestamp:    entry.Timestamp,
			TimestampAgo: timestampAgo,
			Type:         "nudge",
			Actor:        actorName,
			ActorType:    actorType,
			Summary:      fmt.Sprintf("👉 Nudged %s", targetName),
			Details:      reasonText,
			Rig:          rig,
		}

	case "handoff":
		subject, _ := entry.Payload["subject"].(string)
		if subject == "" {
			subject = "work"
		}
		if len(subject) > 40 {
			subject = subject[:37] + "..."
		}
		return &ActivityEvent{
			ID:           fmt.Sprintf("handoff-%s-%s", entry.Timestamp, actorName),
			Timestamp:    entry.Timestamp,
			TimestampAgo: timestampAgo,
			Type:         "handoff",
			Actor:        actorName,
			ActorType:    actorType,
			Summary:      fmt.Sprintf("🔄 Handed off: %s", subject),
			Rig:          rig,
		}

	case "done":
		branch, _ := entry.Payload["branch"].(string)
		beadID, _ := entry.Payload["bead"].(string)
		summary := "✅ Completed work"
		if branch != "" {
			summary = fmt.Sprintf("✅ Completed %s", branch)
		} else if beadID != "" {
			summary = fmt.Sprintf("✅ Completed %s", beadID)
		}
		return &ActivityEvent{
			ID:           fmt.Sprintf("done-%s-%s", entry.Timestamp, actorName),
			Timestamp:    entry.Timestamp,
			TimestampAgo: timestampAgo,
			Type:         "done",
			Actor:        actorName,
			ActorType:    actorType,
			Summary:      summary,
			Rig:          rig,
		}

	case "kill":
		targetName := target
		if targetName == "" {
			targetName = polecat
		}
		return &ActivityEvent{
			ID:           fmt.Sprintf("kill-%s-%s", entry.Timestamp, targetName),
			Timestamp:    entry.Timestamp,
			TimestampAgo: timestampAgo,
			Type:         "kill",
			Actor:        actorName,
			ActorType:    actorType,
			Summary:      fmt.Sprintf("💀 Killed %s", targetName),
			Details:      reason,
			Rig:          rig,
		}

	case "escalation_sent":
		to, _ := entry.Payload["to"].(string)
		return &ActivityEvent{
			ID:           fmt.Sprintf("escalation-%s-%s", entry.Timestamp, actorName),
			Timestamp:    entry.Timestamp,
			TimestampAgo: timestampAgo,
			Type:         "escalation",
			Actor:        actorName,
			ActorType:    actorType,
			Summary:      fmt.Sprintf("⚠️ Escalated %s to %s", target, to),
			Details:      reason,
			Rig:          rig,
		}

	case "boot":
		agents, _ := entry.Payload["agents"].([]interface{})
		agentCount := len(agents)
		return &ActivityEvent{
			ID:           fmt.Sprintf("boot-%s-%s", entry.Timestamp, rig),
			Timestamp:    entry.Timestamp,
			TimestampAgo: timestampAgo,
			Type:         "boot",
			Actor:        "system",
			ActorType:    "system",
			Summary:      fmt.Sprintf("🚀 Booted rig %s with %d agents", rig, agentCount),
			Rig:          rig,
		}

	case "halt":
		return &ActivityEvent{
			ID:           fmt.Sprintf("halt-%s", entry.Timestamp),
			Timestamp:    entry.Timestamp,
			TimestampAgo: timestampAgo,
			Type:         "halt",
			Actor:        actorName,
			ActorType:    actorType,
			Summary:      "🛑 Halted town services",
		}

	case "sling":
		beadID, _ := entry.Payload["bead"].(string)
		target, _ := entry.Payload["target"].(string)
		return &ActivityEvent{
			ID:           fmt.Sprintf("sling-%s-%s", entry.Timestamp, beadID),
			Timestamp:    entry.Timestamp,
			TimestampAgo: timestampAgo,
			Type:         "sling",
			Actor:        actorName,
			ActorType:    actorType,
			Summary:      fmt.Sprintf("📦 Slung work to %s", target),
			Details:      beadID,
		}

	case "mail":
		to, _ := entry.Payload["to"].(string)
		subject, _ := entry.Payload["subject"].(string)
		if len(subject) > 40 {
			subject = subject[:37] + "..."
		}
		return &ActivityEvent{
			ID:           fmt.Sprintf("mail-%s-%s", entry.Timestamp, to),
			Timestamp:    entry.Timestamp,
			TimestampAgo: timestampAgo,
			Type:         "mail",
			Actor:        actorName,
			ActorType:    actorType,
			Summary:      fmt.Sprintf("📬 Sent mail to %s", to),
			Details:      subject,
		}

	case "patrol_complete":
		polecatCount, _ := entry.Payload["polecat_count"].(float64)
		message, _ := entry.Payload["message"].(string)
		summary := fmt.Sprintf("🔍 Patrol complete (%d polecats)", int(polecatCount))
		return &ActivityEvent{
			ID:           fmt.Sprintf("patrol-%s-%s", entry.Timestamp, actorName),
			Timestamp:    entry.Timestamp,
			TimestampAgo: timestampAgo,
			Type:         "patrol",
			Actor:        actorName,
			ActorType:    actorType,
			Summary:      summary,
			Details:      message,
			Rig:          rig,
		}

	default:
		// Skip uninteresting event types
		return nil
	}
}

// formatTimeAgo formats a time as a human-readable "X ago" string.
func formatTimeAgo(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		m := int(d.Minutes())
		if m == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", m)
	case d < 24*time.Hour:
		h := int(d.Hours())
		if h == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", h)
	default:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	}
}

// fetchMainBranchCommits gets recent commits from each rig's main branch (via refinery clone).
// This catches direct pushes that don't go through GitHub PRs.
func (f *LiveConvoyFetcher) fetchMainBranchCommits(limit int) []ActivityEvent {
	var events []ActivityEvent

	rigsConfigPath := filepath.Join(f.townRoot, "mayor", "rigs.json")
	rigsConfig, err := config.LoadRigsConfig(rigsConfigPath)
	if err != nil {
		return events
	}

	for rigName := range rigsConfig.Rigs {
		// Check refinery clone path: townRoot/rigName/refinery/rig
		refineryPath := filepath.Join(f.townRoot, rigName, "refinery", "rig")

		// Fetch latest from origin first
		fetchCmd := exec.Command("git", "fetch", "origin")
		fetchCmd.Dir = refineryPath
		_ = fetchCmd.Run() // Ignore errors, proceed with potentially stale data

		// Get recent commits on origin/main (last hour)
		cmd := exec.Command("git", "log", "origin/main", "--since=1 hour ago", "--format=%H|%s|%ar|%aI|%an", "-n", fmt.Sprintf("%d", limit))
		cmd.Dir = refineryPath
		var stdout bytes.Buffer
		cmd.Stdout = &stdout
		if err := cmd.Run(); err != nil {
			continue
		}

		lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
		for _, line := range lines {
			if line == "" {
				continue
			}
			parts := strings.SplitN(line, "|", 5)
			if len(parts) < 5 {
				continue
			}

			hash := parts[0][:8]
			msg := parts[1]
			if len(msg) > 50 {
				msg = msg[:47] + "..."
			}
			author := parts[4]

			events = append(events, ActivityEvent{
				ID:           fmt.Sprintf("main-%s-%s", rigName, hash),
				Timestamp:    parts[3],
				TimestampAgo: parts[2],
				Type:         "main_commit",
				Actor:        author,
				ActorType:    "refinery",
				Summary:      fmt.Sprintf("Pushed to main: \"%s\"", msg),
				Details:      rigName,
				Rig:          rigName,
			})
		}
	}

	return events
}

// FetchHQAgentDetail returns expanded information for an HQ agent session.
func (f *LiveConvoyFetcher) FetchHQAgentDetail(sessionID string) (*HQAgentDetail, error) {
	// Validate session ID format
	if !strings.HasPrefix(sessionID, "hq-") {
		return nil, fmt.Errorf("invalid HQ session ID format: %s", sessionID)
	}

	// Extract agent name from session ID (hq-mayor -> mayor)
	agentName := strings.TrimPrefix(sessionID, "hq-")

	// Get session activity and creation time
	cmd := exec.Command("tmux", "list-sessions", "-F", "#{session_name}|#{window_activity}|#{session_created}",
		"-f", fmt.Sprintf("#{==:#{session_name},%s}", sessionID))
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}

	output := strings.TrimSpace(stdout.String())
	if output == "" {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}

	parts := strings.Split(output, "|")
	if len(parts) < 3 {
		return nil, fmt.Errorf("unexpected tmux output format")
	}

	// Parse activity and created timestamps
	var activityUnix, createdUnix int64
	fmt.Sscanf(parts[1], "%d", &activityUnix)
	fmt.Sscanf(parts[2], "%d", &createdUnix)

	activityTime := time.Unix(activityUnix, 0)
	createdTime := time.Unix(createdUnix, 0)
	uptime := formatDuration(time.Since(createdTime))

	// Get terminal lines
	terminalLines := f.getPolecatTerminalLines(sessionID, 15)

	// Get status hint
	statusHint := f.getStatusHintWithDefault(sessionID)

	return &HQAgentDetail{
		HQAgentRow: HQAgentRow{
			Name:         agentName,
			SessionID:    sessionID,
			Status:       "running",
			LastActivity: activity.Calculate(activityTime),
			StatusHint:   statusHint,
		},
		Uptime:        uptime,
		TerminalLines: terminalLines,
	}, nil
}

// FetchHQAgents returns status of town-level agents (Mayor, Deacon).
func (f *LiveConvoyFetcher) FetchHQAgents() ([]HQAgentRow, error) {
	var agents []HQAgentRow

	// Define the HQ agent sessions to look for
	hqAgents := []struct {
		name      string
		sessionID string
	}{
		{"mayor", "hq-mayor"},
		{"deacon", "hq-deacon"},
	}

	for _, agent := range hqAgents {
		row := HQAgentRow{
			Name:      agent.name,
			SessionID: agent.sessionID,
			Status:    "stopped",
		}

		// Query tmux for this session
		cmd := exec.Command("tmux", "list-sessions", "-F", "#{session_name}|#{window_activity}",
			"-f", fmt.Sprintf("#{==:#{session_name},%s}", agent.sessionID))
		var stdout bytes.Buffer
		cmd.Stdout = &stdout
		if err := cmd.Run(); err == nil {
			output := strings.TrimSpace(stdout.String())
			if output != "" {
				row.Status = "running"

				// Parse activity timestamp
				parts := strings.Split(output, "|")
				if len(parts) >= 2 {
					var activityUnix int64
					if _, err := fmt.Sscanf(parts[1], "%d", &activityUnix); err == nil && activityUnix > 0 {
						activityTime := time.Unix(activityUnix, 0)
						row.LastActivity = activity.Calculate(activityTime)
					}
				}

				// Get status hint from pane
				row.StatusHint = f.getStatusHintWithDefault(agent.sessionID)
			}
		}

		// If stopped, set default activity
		if row.Status == "stopped" {
			row.LastActivity = activity.Info{
				FormattedAge: "offline",
				ColorClass:   activity.ColorUnknown,
			}
		}

		agents = append(agents, row)
	}

	return agents, nil
}

// fetchCommitEvents gets recent commits from polecat worktrees.
func (f *LiveConvoyFetcher) fetchCommitEvents(limit int) []ActivityEvent {
	var events []ActivityEvent

	// Get all polecats
	polecats, err := f.FetchPolecats()
	if err != nil {
		return events
	}

	for _, polecat := range polecats {
		// Get rig path: townRoot/rigName/polecats/polecatName/rigName
		worktreePath := filepath.Join(f.townRoot, polecat.Rig, "polecats", polecat.Name, polecat.Rig)

		// Get recent commits (last hour)
		cmd := exec.Command("git", "log", "--since=1 hour ago", "--format=%H|%s|%ar|%aI", "-n", "5")
		cmd.Dir = worktreePath
		var stdout bytes.Buffer
		cmd.Stdout = &stdout
		if err := cmd.Run(); err != nil {
			continue
		}

		lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
		for _, line := range lines {
			if line == "" {
				continue
			}
			parts := strings.SplitN(line, "|", 4)
			if len(parts) < 4 {
				continue
			}

			hash := parts[0][:8]
			msg := parts[1]
			if len(msg) > 50 {
				msg = msg[:47] + "..."
			}

			events = append(events, ActivityEvent{
				ID:           fmt.Sprintf("commit-%s", hash),
				Timestamp:    parts[3],
				TimestampAgo: parts[2],
				Type:         "commit",
				Actor:        polecat.Name,
				ActorType:    "polecat",
				Summary:      fmt.Sprintf("Committed: \"%s\"", msg),
				Rig:          polecat.Rig,
			})
		}
	}

	return events
}

// FetchInternalMRs fetches open MR beads from all registered rigs.
// These are the internal merge requests tracked by beads (what mayor reports on),
// as opposed to GitHub PRs which are fetched by FetchMergeQueue.
// MRs are identified by the gt:merge-request label, not by type.
func (f *LiveConvoyFetcher) FetchInternalMRs() ([]InternalMRRow, error) {
	// Load registered rigs from config
	rigsConfigPath := filepath.Join(f.townRoot, "mayor", "rigs.json")
	rigsConfig, err := config.LoadRigsConfig(rigsConfigPath)
	if err != nil {
		return nil, fmt.Errorf("loading rigs config: %w", err)
	}

	var result []InternalMRRow

	for rigName := range rigsConfig.Rigs {
		// Query beads for this rig's open merge-request issues
		// bd must be run from the rig directory (it finds .beads automatically)
		rigPath := filepath.Join(f.townRoot, rigName)
		beadsPath := filepath.Join(rigPath, ".beads")

		// Check if beads database exists for this rig
		if _, err := os.Stat(beadsPath); os.IsNotExist(err) {
			continue
		}

		// Query for open merge requests using label (not type - see #816)
		// MRs created by `gt done` have type=task but gt:merge-request label
		cmd := exec.Command("bd", "list", "--label", "gt:merge-request", "--status=open", "--json")
		cmd.Dir = rigPath
		var stdout bytes.Buffer
		cmd.Stdout = &stdout
		if err := cmd.Run(); err != nil {
			continue // Non-fatal: skip this rig
		}

		var mrs []struct {
			ID          string `json:"id"`
			Title       string `json:"title"`
			Status      string `json:"status"`
			Description string `json:"description"`
		}
		if err := json.Unmarshal(stdout.Bytes(), &mrs); err != nil {
			continue
		}

		for _, mr := range mrs {
			// Parse MR fields from description
			fields := parseMRDescription(mr.Description)

			row := InternalMRRow{
				ID:          mr.ID,
				Rig:         rigName,
				Title:       mr.Title,
				Status:      mr.Status,
				Branch:      fields.Branch,
				Target:      fields.Target,
				Worker:      fields.Worker,
				SourceIssue: fields.SourceIssue,
			}

			// Determine color class based on status
			if mr.Status == "in_progress" {
				row.ColorClass = "mq-yellow"
			} else {
				row.ColorClass = "mq-green"
			}

			result = append(result, row)
		}

		// Also check for in_progress MRs
		cmd = exec.Command("bd", "list", "--label", "gt:merge-request", "--status=in_progress", "--json")
		cmd.Dir = rigPath
		stdout.Reset()
		cmd.Stdout = &stdout
		if err := cmd.Run(); err == nil {
			var inProgressMRs []struct {
				ID          string `json:"id"`
				Title       string `json:"title"`
				Status      string `json:"status"`
				Description string `json:"description"`
			}
			if err := json.Unmarshal(stdout.Bytes(), &inProgressMRs); err == nil {
				for _, mr := range inProgressMRs {
					fields := parseMRDescription(mr.Description)
					row := InternalMRRow{
						ID:          mr.ID,
						Rig:         rigName,
						Title:       mr.Title,
						Status:      mr.Status,
						Branch:      fields.Branch,
						Target:      fields.Target,
						Worker:      fields.Worker,
						SourceIssue: fields.SourceIssue,
						ColorClass:  "mq-yellow",
					}
					result = append(result, row)
				}
			}
		}
	}

	return result, nil
}

// mrDescFields holds parsed MR description fields.
type mrDescFields struct {
	Branch      string
	Target      string
	Worker      string
	SourceIssue string
}

// parseMRDescription extracts fields from MR description.
// Format: key: value lines (e.g., "branch: polecat/Nux/gt-xyz")
func parseMRDescription(desc string) mrDescFields {
	var fields mrDescFields
	for _, line := range strings.Split(desc, "\n") {
		line = strings.TrimSpace(line)
		colonIdx := strings.Index(line, ":")
		if colonIdx == -1 {
			continue
		}
		key := strings.TrimSpace(strings.ToLower(line[:colonIdx]))
		value := strings.TrimSpace(line[colonIdx+1:])
		switch key {
		case "branch":
			fields.Branch = value
		case "target":
			fields.Target = value
		case "worker":
			fields.Worker = value
		case "source_issue", "source-issue", "sourceissue":
			fields.SourceIssue = value
		}
	}
	return fields
}
