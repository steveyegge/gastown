package doctor

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/doltserver"
)

// CheckMisclassifiedWisps detects issues that should be marked as wisps but aren't.
// Wisps are ephemeral issues for operational workflows (patrols, MRs, mail, agents).
// This check finds issues that have wisp characteristics but lack the wisp:true flag.
//
// Detection prefers Dolt (live DB via bd sql --csv) over JSONL, falling back to
// JSONL when the DB is unreachable.
type CheckMisclassifiedWisps struct {
	FixableCheck
	misclassified     []misclassifiedWisp
	misclassifiedRigs map[string]int // rig -> count
}

type misclassifiedWisp struct {
	rigName string
	id      string
	title   string
	reason  string
}

// NewCheckMisclassifiedWisps creates a new misclassified wisp check.
func NewCheckMisclassifiedWisps() *CheckMisclassifiedWisps {
	return &CheckMisclassifiedWisps{
		FixableCheck: FixableCheck{
			BaseCheck: BaseCheck{
				CheckName:        "misclassified-wisps",
				CheckDescription: "Detect issues that should be wisps but aren't marked as ephemeral",
				CheckCategory:    CategoryCleanup,
			},
		},
		misclassifiedRigs: make(map[string]int),
	}
}

// Run checks for misclassified wisps in each rig.
// Prefers Dolt queries for accuracy; falls back to JSONL if Dolt is unavailable.
func (c *CheckMisclassifiedWisps) Run(ctx *CheckContext) *CheckResult {
	c.misclassified = nil
	c.misclassifiedRigs = make(map[string]int)

	// Try Dolt-first detection via ListDatabases (matches NullAssigneeCheck pattern).
	databases, dbErr := doltserver.ListDatabases(ctx.TownRoot)
	useDolt := dbErr == nil && len(databases) > 0

	var details []string
	var totalProbeErrors int

	if useDolt {
		// Dolt path covers rig databases only (no town-level beads).
		// Town-level beads are rare and covered by the JSONL fallback path.
		for _, db := range databases {
			// The "hq" database lives at the town root itself, not townRoot/hq.
			rigDir := filepath.Join(ctx.TownRoot, db)
			if db == "hq" {
				rigDir = ctx.TownRoot
			}
			found, probeErrors := c.findMisclassifiedWispsDolt(rigDir, db)
			totalProbeErrors += probeErrors
			if len(found) > 0 {
				c.misclassified = append(c.misclassified, found...)
				c.misclassifiedRigs[db] = len(found)
				details = append(details, fmt.Sprintf("%s: %d misclassified wisp(s)", db, len(found)))
			}
		}
	} else {
		// Fallback: JSONL-based detection (original path).
		rigs, err := discoverRigs(ctx.TownRoot)
		if err != nil {
			return &CheckResult{
				Name:    c.Name(),
				Status:  StatusError,
				Message: "Failed to discover rigs",
				Details: []string{err.Error()},
			}
		}

		if len(rigs) == 0 {
			return &CheckResult{
				Name:    c.Name(),
				Status:  StatusOK,
				Message: "No rigs configured",
			}
		}

		for _, rigName := range rigs {
			rigPath := filepath.Join(ctx.TownRoot, rigName)
			found, probeErrors := c.findMisclassifiedWispsJSONL(rigPath, rigName)
			totalProbeErrors += probeErrors
			if len(found) > 0 {
				c.misclassified = append(c.misclassified, found...)
				c.misclassifiedRigs[rigName] = len(found)
				details = append(details, fmt.Sprintf("%s: %d misclassified wisp(s)", rigName, len(found)))
			}
		}

		// Also check town-level beads (JSONL fallback only).
		townFound, townProbeErrors := c.findMisclassifiedWispsJSONL(ctx.TownRoot, "town")
		totalProbeErrors += townProbeErrors
		if len(townFound) > 0 {
			c.misclassified = append(c.misclassified, townFound...)
			c.misclassifiedRigs["town"] = len(townFound)
			details = append(details, fmt.Sprintf("town: %d misclassified wisp(s)", len(townFound)))
		}
	}

	if totalProbeErrors > 0 {
		details = append(details, fmt.Sprintf("%d DB probe(s) failed — some candidates may have been skipped", totalProbeErrors))
	}

	total := len(c.misclassified)
	if total > 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusWarning,
			Message: fmt.Sprintf("%d issue(s) should be marked as wisps", total),
			Details: details,
			FixHint: "Run 'gt doctor --fix' to purge from issues and migrate to wisps table",
		}
	}

	if totalProbeErrors > 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusWarning,
			Message: "No misclassified wisps found (some DB probes failed)",
			Details: details,
		}
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusOK,
		Message: "No misclassified wisps found",
	}
}

// findMisclassifiedWispsDolt queries the live Dolt DB for non-ephemeral, non-closed issues
// and checks each against shouldBeWisp(). Returns found wisps and probe error count.
func (c *CheckMisclassifiedWisps) findMisclassifiedWispsDolt(rigDir, rigName string) ([]misclassifiedWisp, int) {
	// Query issues: non-closed, non-ephemeral.
	issueQuery := `SELECT id, title, status, issue_type FROM issues WHERE status != 'closed' AND (ephemeral = 0 OR ephemeral IS NULL)`
	cmd := exec.Command("bd", "sql", "--csv", issueQuery) //nolint:gosec // G204: query is a constant
	cmd.Dir = rigDir
	issueOutput, err := cmd.CombinedOutput()
	if err != nil {
		return nil, 1 // DB unavailable for this rig
	}

	// Parse issues CSV.
	type issueRow struct {
		id, title, status, issueType string
	}
	issueReader := csv.NewReader(strings.NewReader(string(issueOutput)))
	issueRecords, err := issueReader.ReadAll()
	if err != nil || len(issueRecords) < 2 {
		return nil, 0 // No issues or parse error
	}
	issues := make([]issueRow, 0, len(issueRecords)-1)
	for _, rec := range issueRecords[1:] {
		if len(rec) < 4 {
			continue
		}
		issues = append(issues, issueRow{
			id:        strings.TrimSpace(rec[0]),
			title:     strings.TrimSpace(rec[1]),
			status:    strings.TrimSpace(rec[2]),
			issueType: strings.TrimSpace(rec[3]),
		})
	}

	// Query labels for non-closed, non-ephemeral issues.
	labelQuery := `SELECT l.issue_id, l.label FROM labels l JOIN issues i ON l.issue_id = i.id WHERE i.status != 'closed' AND (i.ephemeral = 0 OR i.ephemeral IS NULL)`
	labelCmd := exec.Command("bd", "sql", "--csv", labelQuery) //nolint:gosec // G204: query is a constant
	labelCmd.Dir = rigDir
	labelOutput, _ := labelCmd.CombinedOutput()

	// Build label map: issue_id -> []label.
	labelMap := make(map[string][]string)
	if len(labelOutput) > 0 {
		labelReader := csv.NewReader(strings.NewReader(string(labelOutput)))
		labelRecords, _ := labelReader.ReadAll()
		for _, rec := range labelRecords[1:] {
			if len(rec) < 2 {
				continue
			}
			id := strings.TrimSpace(rec[0])
			label := strings.TrimSpace(rec[1])
			labelMap[id] = append(labelMap[id], label)
		}
	}

	// Check each issue against shouldBeWisp.
	var found []misclassifiedWisp
	for _, issue := range issues {
		labels := labelMap[issue.id]
		if reason := c.shouldBeWisp(issue.id, issue.title, issue.issueType, labels); reason != "" {
			found = append(found, misclassifiedWisp{
				rigName: rigName,
				id:      issue.id,
				title:   issue.title,
				reason:  reason,
			})
		}
	}

	return found, 0
}

// findMisclassifiedWispsJSONL finds misclassified wisps from JSONL files (fallback path).
// Returns the found misclassified wisps and the number of DB probe errors encountered.
func (c *CheckMisclassifiedWisps) findMisclassifiedWispsJSONL(path string, rigName string) ([]misclassifiedWisp, int) {
	beadsDir := beads.ResolveBeadsDir(path)
	issuesPath := filepath.Join(beadsDir, "issues.jsonl")
	file, err := os.Open(issuesPath)
	if err != nil {
		return nil, 0 // No issues file
	}
	defer file.Close()

	var found []misclassifiedWisp
	var probeErrors int

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var issue struct {
			ID        string   `json:"id"`
			Title     string   `json:"title"`
			Status    string   `json:"status"`
			Type      string   `json:"issue_type"`
			Labels    []string `json:"labels"`
			Ephemeral bool     `json:"ephemeral"`
		}
		if err := json.Unmarshal([]byte(line), &issue); err != nil {
			continue
		}

		// Skip issues already marked as ephemeral/wisps
		if issue.Ephemeral {
			continue
		}

		// Skip closed issues - they're done, no need to reclassify
		if issue.Status == "closed" {
			continue
		}

		// Check for wisp characteristics
		if reason := c.shouldBeWisp(issue.ID, issue.Title, issue.Type, issue.Labels); reason != "" {
			// Verify the current DB state (JSONL may be stale if daemon isn't running)
			open, err := isIssueStillOpen(path, issue.ID)
			if err != nil {
				probeErrors++
				continue
			}
			if open {
				found = append(found, misclassifiedWisp{
					rigName: rigName,
					id:      issue.ID,
					title:   issue.Title,
					reason:  reason,
				})
			}
		}
	}

	return found, probeErrors
}

// isIssueStillOpen verifies an issue is still open/non-ephemeral in the live DB.
// This guards against stale JSONL data when the daemon isn't running and hasn't flushed.
// Uses --allow-stale to survive DB/JSONL drift (consistent with all other bd invocations).
// Returns an error if the probe fails, so callers can track and surface failures.
func isIssueStillOpen(workDir, id string) (bool, error) {
	cmd := exec.Command("bd", "--allow-stale", "show", id, "--json")
	cmd.Dir = workDir
	output, err := cmd.Output()
	if err != nil {
		stderr := ""
		if ee, ok := err.(*exec.ExitError); ok {
			stderr = strings.TrimSpace(string(ee.Stderr))
		}
		// "not found" means the issue was deleted or migrated (e.g. to wisps).
		// Treat as "not open" rather than a probe error.
		if strings.Contains(stderr, "not found") || strings.Contains(string(output), "no issues found") {
			return false, nil
		}
		return false, fmt.Errorf("bd show %s: %v (%s)", id, err, stderr)
	}
	var issues []struct {
		Status    string `json:"status"`
		Ephemeral bool   `json:"ephemeral"`
	}
	if err := json.Unmarshal(output, &issues); err != nil {
		return false, fmt.Errorf("bd show %s: parse error: %v", id, err)
	}
	if len(issues) == 0 {
		return false, fmt.Errorf("bd show %s: empty result", id)
	}
	issue := issues[0]
	return issue.Status != "closed" && !issue.Ephemeral, nil
}

// shouldBeWisp checks if an issue has characteristics indicating it should be a wisp.
// Returns the reason string if it should be a wisp, empty string otherwise.
func (c *CheckMisclassifiedWisps) shouldBeWisp(id, title, issueType string, labels []string) string {
	// Check for merge-request type - these should always be wisps
	if issueType == "merge-request" {
		return "merge-request type should be ephemeral"
	}

	// Agent type is NOT ephemeral: persistent polecats design (c410c10a) stores
	// agent beads in the issues table for durability across polecat lifecycles.
	// Previously flagged as ephemeral (gt-bewatn.9) but that was reversed.

	// Check for event type - session/cost events are operational telemetry
	if issueType == "event" {
		return "event type should be ephemeral"
	}

	// Check for gate type - async coordination gates are ephemeral
	if issueType == "gate" {
		return "gate type should be ephemeral"
	}

	// Check for slot type - exclusive access slots are ephemeral
	if issueType == "slot" {
		return "slot type should be ephemeral"
	}

	// Check for patrol-related labels
	for _, label := range labels {
		if strings.Contains(label, "patrol") {
			return "patrol label indicates ephemeral workflow"
		}
		if label == "gt:mail" || label == "gt:handoff" {
			return "mail/handoff label indicates ephemeral message"
		}
		// gt:agent label is NOT an ephemeral indicator: persistent polecats
		// design (c410c10a) keeps agent beads in the issues table.
		// Previously flagged here but that would undo the migration.
	}

	// Check for formula instance patterns in ID
	// Formula instances typically have IDs like "mol-<formula>-<hash>" or "<formula>.<step>"
	if strings.HasPrefix(id, "mol-") && strings.Contains(id, "-patrol") {
		return "patrol molecule ID pattern"
	}

	// Check for specific title patterns indicating operational work
	lowerTitle := strings.ToLower(title)
	if strings.Contains(lowerTitle, "patrol cycle") ||
		strings.Contains(lowerTitle, "witness patrol") ||
		strings.Contains(lowerTitle, "deacon patrol") ||
		strings.Contains(lowerTitle, "refinery patrol") {
		return "patrol title indicates ephemeral workflow"
	}

	return ""
}

// Fix purges misclassified issues: migrates them to the wisps table and deletes
// from the version-controlled issues table. Falls back to `bd update --ephemeral`
// when the wisps table doesn't exist.
//
// Pattern follows wisps_migrate.go (INSERT IGNORE) + NullAssigneeCheck (bd sql + commit).
func (c *CheckMisclassifiedWisps) Fix(ctx *CheckContext) error {
	if len(c.misclassified) == 0 {
		return nil
	}

	// Group misclassified wisps by rig for batch operations.
	rigBatches := make(map[string][]misclassifiedWisp)
	for _, w := range c.misclassified {
		rigBatches[w.rigName] = append(rigBatches[w.rigName], w)
	}

	var errs []string

	for rigName, batch := range rigBatches {
		var workDir string
		if rigName == "town" {
			workDir = ctx.TownRoot
		} else {
			workDir = filepath.Join(ctx.TownRoot, rigName)
		}

		ids := make([]string, len(batch))
		for i, w := range batch {
			ids[i] = "'" + strings.ReplaceAll(w.id, "'", "''") + "'"
		}
		idList := strings.Join(ids, ", ")

		if err := c.purgeRigBatch(ctx, workDir, rigName, idList); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", rigName, err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("partial fix: %s", strings.Join(errs, "; "))
	}
	return nil
}

// purgeRigBatch migrates a batch of misclassified wisps for one rig:
// 1. Check wisps table exists (fall back to UPDATE if not)
// 2. INSERT IGNORE into wisps
// 3. Copy auxiliary data (labels, comments, events, deps)
// 4. DELETE from issues + auxiliary tables
// 5. Commit to Dolt history
func (c *CheckMisclassifiedWisps) purgeRigBatch(ctx *CheckContext, workDir, rigName, idList string) error {
	// Check if wisps table exists. If not, fall back to setting ephemeral flag.
	hasWisps := bdTableExistsDoctor(workDir, "wisps")
	if !hasWisps {
		// Fallback: just mark ephemeral (original behavior).
		query := fmt.Sprintf("UPDATE issues SET ephemeral = 1 WHERE id IN (%s)", idList)
		if err := execBdSQLWrite(workDir, query); err != nil {
			return fmt.Errorf("ephemeral fallback: %w", err)
		}
		commitMsg := "fix: mark misclassified wisps as ephemeral (gt doctor)"
		_ = doltserver.CommitServerWorkingSet(ctx.TownRoot, rigName, commitMsg)
		return nil
	}

	// Step 1: Migrate issues to wisps table (INSERT IGNORE skips duplicates).
	migrateQuery := fmt.Sprintf(
		"INSERT IGNORE INTO wisps (id, title, description, status, issue_type, agent_state, role_type, rig, hook_bead, role_bead, created_at, updated_at, created_by, owner, assignee, priority, ephemeral, wisp_type, mol_type, metadata) "+
			"SELECT id, title, description, status, issue_type, agent_state, role_type, rig, hook_bead, role_bead, created_at, updated_at, created_by, owner, assignee, priority, 1, wisp_type, mol_type, metadata FROM issues WHERE id IN (%s)",
		idList)
	if err := execBdSQLWrite(workDir, migrateQuery); err != nil {
		return fmt.Errorf("migrate to wisps: %w", err)
	}

	// Step 2: Copy auxiliary data to wisp_* tables.
	auxCopies := []struct {
		table string
		query string
	}{
		{
			table: "wisp_labels",
			query: fmt.Sprintf("INSERT IGNORE INTO wisp_labels (issue_id, label) SELECT l.issue_id, l.label FROM labels l WHERE l.issue_id IN (%s)", idList),
		},
		{
			table: "wisp_comments",
			query: fmt.Sprintf("INSERT IGNORE INTO wisp_comments (issue_id, author, text, created_at) SELECT c.issue_id, c.author, c.text, c.created_at FROM comments c WHERE c.issue_id IN (%s)", idList),
		},
		{
			table: "wisp_events",
			query: fmt.Sprintf("INSERT IGNORE INTO wisp_events (issue_id, event_type, actor, old_value, new_value, comment, created_at) SELECT e.issue_id, e.event_type, e.actor, e.old_value, e.new_value, e.comment, e.created_at FROM events e WHERE e.issue_id IN (%s)", idList),
		},
		{
			table: "wisp_dependencies",
			query: fmt.Sprintf("INSERT IGNORE INTO wisp_dependencies (issue_id, depends_on_id, type, created_at, created_by, metadata, thread_id) SELECT d.issue_id, d.depends_on_id, d.type, d.created_at, d.created_by, d.metadata, d.thread_id FROM dependencies d WHERE d.issue_id IN (%s)", idList),
		},
	}
	for _, aux := range auxCopies {
		if bdTableExistsDoctor(workDir, aux.table) {
			_ = execBdSQLWrite(workDir, aux.query) // Best-effort
		}
	}

	// Step 3: Delete from auxiliary tables first (referential integrity).
	auxDeletes := []string{
		fmt.Sprintf("DELETE FROM labels WHERE issue_id IN (%s)", idList),
		fmt.Sprintf("DELETE FROM comments WHERE issue_id IN (%s)", idList),
		fmt.Sprintf("DELETE FROM events WHERE issue_id IN (%s)", idList),
		fmt.Sprintf("DELETE FROM dependencies WHERE issue_id IN (%s)", idList),
	}
	for _, q := range auxDeletes {
		_ = execBdSQLWrite(workDir, q) // Best-effort: table may not exist
	}

	// Step 4: Delete from issues table.
	deleteQuery := fmt.Sprintf("DELETE FROM issues WHERE id IN (%s)", idList)
	if err := execBdSQLWrite(workDir, deleteQuery); err != nil {
		return fmt.Errorf("delete from issues: %w", err)
	}

	// Step 5: Commit purge to Dolt history.
	commitMsg := "fix: purge misclassified wisps from issues table (gt doctor)"
	if err := doltserver.CommitServerWorkingSet(ctx.TownRoot, rigName, commitMsg); err != nil {
		// Non-fatal: data is already fixed.
		_ = err
	}

	return nil
}

// bdTableExistsDoctor checks if a table exists by attempting to query it.
// Doctor-local wrapper (wisps_migrate.go has its own unexported copy).
func bdTableExistsDoctor(workDir, tableName string) bool {
	cmd := exec.Command("bd", "sql", fmt.Sprintf("SELECT 1 FROM `%s` LIMIT 1", tableName)) //nolint:gosec // G204: tableName is hardcoded
	cmd.Dir = workDir
	err := cmd.Run()
	return err == nil
}
