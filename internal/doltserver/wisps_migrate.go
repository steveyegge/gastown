// Package doltserver - wisps_migrate.go provides migration of agent beads to the wisps table.
//
// The wisps table is a dolt_ignored copy of the issues table schema, used for
// ephemeral operational data (agent beads, patrol wisps, etc.) that should not
// be version-controlled. This migration:
//   1. Creates the wisps table and auxiliary tables (wisp_labels, wisp_comments,
//      wisp_events, wisp_dependencies) if they don't exist
//   2. Copies existing agent beads (issue_type='agent') from issues to wisps
//   3. Copies associated labels, comments, events, and dependencies
//   4. Closes the originals in the issues table
//
// The migration uses `bd sql` for all database operations because the `dolt sql` CLI
// can crash when operating on dolt_ignored tables. The `bd` tool properly connects to
// the running Dolt server and handles the dolt_ignore semantics.
package doltserver

import (
	"fmt"
	"os/exec"
	"strings"
)

// MigrateWispsResult holds migration statistics.
type MigrateWispsResult struct {
	WispsTableCreated bool
	AuxTablesCreated  []string
	AgentsCopied      int
	AgentsClosed      int
	LabelsCopied      int
	CommentsCopied    int
	EventsCopied      int
	DepsCopied        int
}

// MigrateAgentBeadsToWisps creates the wisps table infrastructure and migrates
// existing agent beads from the issues table. Idempotent — safe to run multiple times.
//
// The workDir parameter should point to a directory where `bd` can find the correct
// beads database (typically the rig's .beads directory or a directory with a redirect).
func MigrateAgentBeadsToWisps(townRoot, workDir string, dryRun bool) (*MigrateWispsResult, error) {
	result := &MigrateWispsResult{}

	// Step 1: Ensure bd migrate has been run (sets up dolt_ignore entries)
	if err := bdExec(workDir, "migrate", "--yes"); err != nil {
		// Non-fatal: might already be up to date
		if !strings.Contains(err.Error(), "already") {
			fmt.Printf("  Note: bd migrate returned: %v\n", err)
		}
	}

	// Step 2: Create wisps table if it doesn't exist
	created, err := ensureWispsTable(workDir)
	if err != nil {
		return nil, fmt.Errorf("creating wisps table: %w", err)
	}
	result.WispsTableCreated = created

	// Step 3: Create auxiliary tables
	auxTables, err := ensureWispAuxTables(workDir)
	if err != nil {
		return nil, fmt.Errorf("creating auxiliary tables: %w", err)
	}
	result.AuxTablesCreated = auxTables

	if dryRun {
		cnt, _ := bdSQLCount(workDir, "SELECT COUNT(*) as cnt FROM issues WHERE issue_type = 'agent' AND status = 'open'")
		result.AgentsCopied = cnt
		return result, nil
	}

	// Step 4: Copy agent beads from issues to wisps
	if err := copyAgentBeadsToWisps(workDir, result); err != nil {
		return nil, fmt.Errorf("copying agent beads: %w", err)
	}

	// Step 5: Copy auxiliary data
	if err := copyAuxiliaryData(workDir, result); err != nil {
		return nil, fmt.Errorf("copying auxiliary data: %w", err)
	}

	// Step 6: Close originals in issues table
	if err := closeOriginalAgentBeads(workDir, result); err != nil {
		return nil, fmt.Errorf("closing originals: %w", err)
	}

	return result, nil
}

// bdSQL executes a SQL query via `bd sql`.
func bdSQL(workDir, query string) error {
	cmd := exec.Command("bd", "sql", query)
	cmd.Dir = workDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("bd sql: %s: %w", strings.TrimSpace(string(output)), err)
	}
	return nil
}

// bdSQLCSV executes a SQL query via `bd sql --csv` and returns the output.
func bdSQLCSV(workDir, query string) (string, error) {
	cmd := exec.Command("bd", "sql", "--csv", query)
	cmd.Dir = workDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("bd sql: %s: %w", strings.TrimSpace(string(output)), err)
	}
	return string(output), nil
}

// bdExec executes a bd command.
func bdExec(workDir string, args ...string) error {
	cmd := exec.Command("bd", args...)
	cmd.Dir = workDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("bd %s: %s: %w", strings.Join(args, " "), strings.TrimSpace(string(output)), err)
	}
	return nil
}

// bdSQLCount executes a COUNT query and returns the integer result.
func bdSQLCount(workDir, query string) (int, error) {
	output, err := bdSQLCSV(workDir, query)
	if err != nil {
		return 0, err
	}
	// Parse CSV output: header\nvalue\n
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) < 2 {
		return 0, nil
	}
	cnt := 0
	fmt.Sscanf(strings.TrimSpace(lines[1]), "%d", &cnt)
	return cnt, nil
}

// bdTableExists checks if a table exists by attempting to query it.
func bdTableExists(workDir, tableName string) bool {
	err := bdSQL(workDir, fmt.Sprintf("SELECT 1 FROM `%s` LIMIT 1", tableName))
	return err == nil
}

// ensureWispsTable creates the wisps table with the core columns needed for agent beads.
func ensureWispsTable(workDir string) (bool, error) {
	if bdTableExists(workDir, "wisps") {
		return false, nil
	}

	// Create the wisps table with all columns the bd tool expects.
	// We use individual column definitions instead of CREATE TABLE LIKE because
	// LIKE can cause Dolt server crashes with dolt_ignored tables.
	err := bdSQL(workDir, `CREATE TABLE wisps (
  id varchar(255) NOT NULL,
  content_hash varchar(64),
  title varchar(500) NOT NULL,
  description text NOT NULL,
  design text NOT NULL DEFAULT '',
  acceptance_criteria text NOT NULL DEFAULT '',
  notes text NOT NULL DEFAULT '',
  status varchar(32) NOT NULL DEFAULT 'open',
  priority int NOT NULL DEFAULT 2,
  issue_type varchar(32) NOT NULL DEFAULT 'task',
  assignee varchar(255),
  estimated_minutes int,
  created_at datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  created_by varchar(255) DEFAULT '',
  owner varchar(255) DEFAULT '',
  updated_at datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  closed_at datetime,
  closed_by_session varchar(255) DEFAULT '',
  external_ref varchar(255),
  compaction_level int DEFAULT 0,
  compacted_at datetime,
  compacted_at_commit varchar(64),
  original_size int,
  deleted_at datetime,
  deleted_by varchar(255) DEFAULT '',
  delete_reason text DEFAULT '',
  original_type varchar(32) DEFAULT '',
  sender varchar(255) DEFAULT '',
  ephemeral tinyint(1) DEFAULT 1,
  pinned tinyint(1) DEFAULT 0,
  is_template tinyint(1) DEFAULT 0,
  crystallizes tinyint(1) DEFAULT 0,
  mol_type varchar(32) DEFAULT '',
  work_type varchar(32) DEFAULT 'mutex',
  quality_score double,
  source_system varchar(255) DEFAULT '',
  metadata json,
  source_repo varchar(512) DEFAULT '',
  close_reason text DEFAULT '',
  event_kind varchar(32) DEFAULT '',
  actor varchar(255) DEFAULT '',
  target varchar(255) DEFAULT '',
  payload text DEFAULT '',
  await_type varchar(32) DEFAULT '',
  await_id varchar(255) DEFAULT '',
  timeout_ns bigint DEFAULT 0,
  waiters text DEFAULT '',
  hook_bead varchar(255) DEFAULT '',
  role_bead varchar(255) DEFAULT '',
  agent_state varchar(32) DEFAULT '',
  last_activity datetime,
  role_type varchar(32) DEFAULT '',
  rig varchar(255) DEFAULT '',
  due_at datetime,
  defer_until datetime,
  wisp_type varchar(32) DEFAULT NULL,
  spec_id text,
  PRIMARY KEY (id),
  KEY idx_wisps_status (status),
  KEY idx_wisps_issue_type (issue_type)
)`)
	if err != nil {
		return false, err
	}

	return true, nil
}

// ensureWispAuxTables creates auxiliary tables for wisps.
func ensureWispAuxTables(workDir string) ([]string, error) {
	var created []string

	auxTables := []struct {
		name string
		ddl  string
	}{
		{
			name: "wisp_labels",
			ddl: `CREATE TABLE wisp_labels (
  issue_id varchar(255) NOT NULL,
  label varchar(255) NOT NULL,
  PRIMARY KEY (issue_id, label),
  KEY idx_wisp_labels_label (label)
)`,
		},
		{
			name: "wisp_comments",
			ddl: `CREATE TABLE wisp_comments (
  id bigint NOT NULL AUTO_INCREMENT,
  issue_id varchar(255) NOT NULL,
  author varchar(255) NOT NULL,
  text text NOT NULL,
  created_at datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  KEY idx_wisp_comments_issue (issue_id)
)`,
		},
		{
			name: "wisp_events",
			ddl: `CREATE TABLE wisp_events (
  id bigint NOT NULL AUTO_INCREMENT,
  issue_id varchar(255) NOT NULL,
  event_type varchar(32) NOT NULL,
  actor varchar(255) NOT NULL,
  old_value text,
  new_value text,
  comment text,
  created_at datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  KEY idx_wisp_events_issue (issue_id)
)`,
		},
		{
			name: "wisp_dependencies",
			ddl: `CREATE TABLE wisp_dependencies (
  issue_id varchar(255) NOT NULL,
  depends_on_id varchar(255) NOT NULL,
  type varchar(32) NOT NULL DEFAULT 'blocks',
  created_at datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  created_by varchar(255) NOT NULL DEFAULT '',
  metadata json,
  thread_id varchar(255) DEFAULT '',
  PRIMARY KEY (issue_id, depends_on_id),
  KEY idx_wisp_deps_depends_on (depends_on_id)
)`,
		},
	}

	for _, t := range auxTables {
		if bdTableExists(workDir, t.name) {
			continue
		}
		if err := bdSQL(workDir, t.ddl); err != nil {
			return created, fmt.Errorf("creating %s: %w", t.name, err)
		}
		created = append(created, t.name)
	}

	return created, nil
}

// copyAgentBeadsToWisps inserts agent beads from issues into wisps, skipping duplicates.
func copyAgentBeadsToWisps(workDir string, result *MigrateWispsResult) error {
	// INSERT IGNORE skips rows where the primary key already exists in wisps.
	// We use explicit column list to handle any schema differences.
	err := bdSQL(workDir,
		"INSERT IGNORE INTO wisps (id, title, description, status, issue_type, agent_state, role_type, rig, hook_bead, role_bead, created_at, updated_at, created_by, owner, assignee, priority, ephemeral, wisp_type, mol_type, metadata) "+
			"SELECT id, title, description, status, issue_type, agent_state, role_type, rig, hook_bead, role_bead, created_at, updated_at, created_by, owner, assignee, priority, 1, wisp_type, mol_type, metadata FROM issues WHERE issue_type = 'agent'")
	if err != nil {
		return err
	}

	cnt, _ := bdSQLCount(workDir, "SELECT COUNT(*) as cnt FROM wisps WHERE issue_type = 'agent'")
	result.AgentsCopied = cnt

	return nil
}

// copyAuxiliaryData copies labels, comments, events, and dependencies for agent beads.
func copyAuxiliaryData(workDir string, result *MigrateWispsResult) error {
	// Copy labels
	if err := bdSQL(workDir,
		"INSERT IGNORE INTO wisp_labels (issue_id, label) SELECT l.issue_id, l.label FROM labels l INNER JOIN wisps w ON l.issue_id = w.id"); err != nil {
		// Non-fatal if no matching labels
		if !strings.Contains(err.Error(), "nothing") {
			return fmt.Errorf("copying labels: %w", err)
		}
	}
	cnt, _ := bdSQLCount(workDir, "SELECT COUNT(*) as cnt FROM wisp_labels")
	result.LabelsCopied = cnt

	// Copy comments
	if err := bdSQL(workDir,
		"INSERT IGNORE INTO wisp_comments (issue_id, author, text, created_at) SELECT c.issue_id, c.author, c.text, c.created_at FROM comments c INNER JOIN wisps w ON c.issue_id = w.id"); err != nil {
		if !strings.Contains(err.Error(), "nothing") {
			return fmt.Errorf("copying comments: %w", err)
		}
	}
	cnt, _ = bdSQLCount(workDir, "SELECT COUNT(*) as cnt FROM wisp_comments")
	result.CommentsCopied = cnt

	// Copy events
	if err := bdSQL(workDir,
		"INSERT IGNORE INTO wisp_events (issue_id, event_type, actor, old_value, new_value, comment, created_at) SELECT e.issue_id, e.event_type, e.actor, e.old_value, e.new_value, e.comment, e.created_at FROM events e INNER JOIN wisps w ON e.issue_id = w.id"); err != nil {
		if !strings.Contains(err.Error(), "nothing") {
			return fmt.Errorf("copying events: %w", err)
		}
	}
	cnt, _ = bdSQLCount(workDir, "SELECT COUNT(*) as cnt FROM wisp_events")
	result.EventsCopied = cnt

	// Copy dependencies
	if err := bdSQL(workDir,
		"INSERT IGNORE INTO wisp_dependencies (issue_id, depends_on_id, type, created_at, created_by, metadata, thread_id) SELECT d.issue_id, d.depends_on_id, d.type, d.created_at, d.created_by, d.metadata, d.thread_id FROM dependencies d INNER JOIN wisps w ON d.issue_id = w.id"); err != nil {
		if !strings.Contains(err.Error(), "nothing") {
			return fmt.Errorf("copying dependencies: %w", err)
		}
	}
	cnt, _ = bdSQLCount(workDir, "SELECT COUNT(*) as cnt FROM wisp_dependencies")
	result.DepsCopied = cnt

	return nil
}

// closeOriginalAgentBeads closes the original agent beads in the issues table.
func closeOriginalAgentBeads(workDir string, result *MigrateWispsResult) error {
	// Close all open agent beads. We don't use a cross-table subquery because
	// that can crash the Dolt server when mixing regular and dolt_ignored tables.
	if err := bdSQL(workDir,
		"UPDATE issues SET status = 'closed', closed_at = NOW() WHERE issue_type = 'agent' AND status = 'open'"); err != nil {
		return err
	}

	cnt, _ := bdSQLCount(workDir, "SELECT COUNT(*) as cnt FROM issues WHERE issue_type = 'agent' AND status = 'closed'")
	result.AgentsClosed = cnt

	return nil
}
