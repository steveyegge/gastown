// Package doltserver - wl_commons.go provides wl-commons (Wasteland) database operations.
//
// The wl-commons database is the shared wanted board for the Wasteland federation.
// Phase 1 (wild-west mode): direct writes to main branch via the local Dolt server.
package doltserver

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// WLCommonsDB is the database name for the wl-commons shared wanted board.
const WLCommonsDB = "wl_commons"

// WLCommonsStore abstracts wl-commons database operations.
type WLCommonsStore interface {
	EnsureDB() error
	DatabaseExists(dbName string) bool
	InsertWanted(item *WantedItem) error
	ClaimWanted(wantedID, rigHandle string) error
	SubmitCompletion(completionID, wantedID, rigHandle, evidence string) error
	QueryWanted(wantedID string) (*WantedItem, error)
}

// WLCommons implements WLCommonsStore using the real Dolt server.
type WLCommons struct{ townRoot string }

// NewWLCommons creates a WLCommonsStore backed by the real Dolt server.
func NewWLCommons(townRoot string) *WLCommons { return &WLCommons{townRoot: townRoot} }

func (w *WLCommons) EnsureDB() error           { return EnsureWLCommons(w.townRoot) }
func (w *WLCommons) DatabaseExists(db string) bool { return DatabaseExists(w.townRoot, db) }
func (w *WLCommons) InsertWanted(item *WantedItem) error { return InsertWanted(w.townRoot, item) }
func (w *WLCommons) ClaimWanted(wantedID, rigHandle string) error {
	return ClaimWanted(w.townRoot, wantedID, rigHandle)
}
func (w *WLCommons) SubmitCompletion(completionID, wantedID, rigHandle, evidence string) error {
	return SubmitCompletion(w.townRoot, completionID, wantedID, rigHandle, evidence)
}
func (w *WLCommons) QueryWanted(wantedID string) (*WantedItem, error) {
	return QueryWanted(w.townRoot, wantedID)
}

// WantedItem represents a row in the wanted table.
type WantedItem struct {
	ID              string
	Title           string
	Description     string
	Project         string
	Type            string
	Priority        int
	Tags            []string
	PostedBy        string
	ClaimedBy       string
	Status          string
	EffortLevel     string
	SandboxRequired bool
}

// isNothingToCommit returns true if the error indicates DOLT_COMMIT found no
// changes to commit. This happens when a conditional UPDATE matched 0 rows,
// leaving the working set unchanged.
func isNothingToCommit(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "nothing to commit")
}

// EscapeSQL escapes backslashes and single quotes for SQL string literals.
// Dolt (MySQL-compatible) treats \ as an escape character, so a trailing
// backslash in user input would escape the closing quote and break the query.
func EscapeSQL(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	return strings.ReplaceAll(s, "'", "''")
}

// GenerateWantedID generates a unique wanted item ID in the format w-<10-char-hash>.
func GenerateWantedID(title string) string {
	randomBytes := make([]byte, 8)
	_, _ = rand.Read(randomBytes)

	input := fmt.Sprintf("%s:%d:%x", title, time.Now().UnixNano(), randomBytes)
	hash := sha256.Sum256([]byte(input))
	hashStr := hex.EncodeToString(hash[:])[:10]

	return fmt.Sprintf("w-%s", hashStr)
}

// EnsureWLCommons ensures the wl-commons database exists and has the correct schema.
func EnsureWLCommons(townRoot string) error {
	config := DefaultConfig(townRoot)
	dbDir := filepath.Join(config.DataDir, WLCommonsDB)

	if _, err := os.Stat(filepath.Join(dbDir, ".dolt")); err == nil {
		return nil
	}

	_, created, err := InitRig(townRoot, WLCommonsDB)
	if err != nil {
		return fmt.Errorf("creating wl-commons database: %w", err)
	}

	if !created {
		return nil
	}

	if err := initWLCommonsSchema(townRoot); err != nil {
		return fmt.Errorf("initializing wl-commons schema: %w", err)
	}

	return nil
}

func initWLCommonsSchema(townRoot string) error {
	schema := fmt.Sprintf(`USE %s;

CREATE TABLE IF NOT EXISTS _meta (
    %s VARCHAR(64) PRIMARY KEY,
    value TEXT
);

INSERT IGNORE INTO _meta (%s, value) VALUES ('schema_version', '1.0');
INSERT IGNORE INTO _meta (%s, value) VALUES ('wasteland_name', 'Gas Town Wasteland');

CREATE TABLE IF NOT EXISTS rigs (
    handle VARCHAR(255) PRIMARY KEY,
    display_name VARCHAR(255),
    dolthub_org VARCHAR(255),
    hop_uri VARCHAR(512),
    owner_email VARCHAR(255),
    gt_version VARCHAR(32),
    trust_level INT DEFAULT 0,
    registered_at TIMESTAMP,
    last_seen TIMESTAMP,
    rig_type VARCHAR(16) DEFAULT 'human',
    parent_rig VARCHAR(255)
);

CREATE TABLE IF NOT EXISTS wanted (
    id VARCHAR(64) PRIMARY KEY,
    title TEXT NOT NULL,
    description TEXT,
    project VARCHAR(64),
    type VARCHAR(32),
    priority INT DEFAULT 2,
    tags JSON,
    posted_by VARCHAR(255),
    claimed_by VARCHAR(255),
    status VARCHAR(32) DEFAULT 'open',
    effort_level VARCHAR(16) DEFAULT 'medium',
    evidence_url TEXT,
    sandbox_required TINYINT(1) DEFAULT 0,
    sandbox_scope JSON,
    sandbox_min_tier VARCHAR(32),
    created_at TIMESTAMP,
    updated_at TIMESTAMP
);

CREATE TABLE IF NOT EXISTS completions (
    id VARCHAR(64) PRIMARY KEY,
    wanted_id VARCHAR(64),
    completed_by VARCHAR(255),
    evidence TEXT,
    validated_by VARCHAR(255),
    stamp_id VARCHAR(64),
    parent_completion_id VARCHAR(64),
    block_hash VARCHAR(64),
    hop_uri VARCHAR(512),
    completed_at TIMESTAMP,
    validated_at TIMESTAMP
);

CREATE TABLE IF NOT EXISTS stamps (
    id VARCHAR(64) PRIMARY KEY,
    author VARCHAR(255) NOT NULL,
    subject VARCHAR(255) NOT NULL,
    valence JSON NOT NULL,
    confidence FLOAT DEFAULT 1,
    severity VARCHAR(16) DEFAULT 'leaf',
    context_id VARCHAR(64),
    context_type VARCHAR(32),
    skill_tags JSON,
    message TEXT,
    prev_stamp_hash VARCHAR(64),
    block_hash VARCHAR(64),
    hop_uri VARCHAR(512),
    created_at TIMESTAMP,
    CHECK (NOT(author = subject))
);

CREATE TABLE IF NOT EXISTS badges (
    id VARCHAR(64) PRIMARY KEY,
    rig_handle VARCHAR(255),
    badge_type VARCHAR(64),
    awarded_at TIMESTAMP,
    evidence TEXT
);

CREATE TABLE IF NOT EXISTS chain_meta (
    chain_id VARCHAR(64) PRIMARY KEY,
    chain_type VARCHAR(32),
    parent_chain_id VARCHAR(64),
    hop_uri VARCHAR(512),
    dolt_database VARCHAR(255),
    created_at TIMESTAMP
);

CALL DOLT_ADD('-A');
CALL DOLT_COMMIT('--allow-empty', '-m', 'Initialize wl-commons schema v1.0');
`, WLCommonsDB,
		backtickKey(), backtickKey(), backtickKey())

	return doltSQLScriptWithRetry(townRoot, schema)
}

func backtickKey() string {
	return "`key`"
}

// InsertWanted inserts a new wanted item into the wl-commons database.
func InsertWanted(townRoot string, item *WantedItem) error {
	if item.ID == "" {
		return fmt.Errorf("wanted item ID cannot be empty")
	}
	if item.Title == "" {
		return fmt.Errorf("wanted item title cannot be empty")
	}

	now := time.Now().UTC().Format("2006-01-02 15:04:05")

	tagsJSON := "NULL"
	if len(item.Tags) > 0 {
		escaped := make([]string, len(item.Tags))
		for i, t := range item.Tags {
			t = strings.ReplaceAll(t, `\`, `\\`)
			t = strings.ReplaceAll(t, `"`, `\"`)
			t = strings.ReplaceAll(t, "'", "''")
			escaped[i] = t
		}
		tagsJSON = fmt.Sprintf("'[\"%s\"]'", strings.Join(escaped, `","`))
	}

	descField := "NULL"
	if item.Description != "" {
		descField = fmt.Sprintf("'%s'", EscapeSQL(item.Description))
	}
	projectField := "NULL"
	if item.Project != "" {
		projectField = fmt.Sprintf("'%s'", EscapeSQL(item.Project))
	}
	typeField := "NULL"
	if item.Type != "" {
		typeField = fmt.Sprintf("'%s'", EscapeSQL(item.Type))
	}
	postedByField := "NULL"
	if item.PostedBy != "" {
		postedByField = fmt.Sprintf("'%s'", EscapeSQL(item.PostedBy))
	}
	effortField := "'medium'"
	if item.EffortLevel != "" {
		effortField = fmt.Sprintf("'%s'", EscapeSQL(item.EffortLevel))
	}
	status := "'open'"
	if item.Status != "" {
		status = fmt.Sprintf("'%s'", EscapeSQL(item.Status))
	}

	script := fmt.Sprintf(`USE %s;

INSERT INTO wanted (id, title, description, project, type, priority, tags, posted_by, status, effort_level, created_at, updated_at)
VALUES ('%s', '%s', %s, %s, %s, %d, %s, %s, %s, %s, '%s', '%s');

CALL DOLT_ADD('-A');
CALL DOLT_COMMIT('-m', 'wl post: %s');
`,
		WLCommonsDB,
		EscapeSQL(item.ID), EscapeSQL(item.Title), descField, projectField, typeField,
		item.Priority, tagsJSON, postedByField, status, effortField,
		now, now,
		EscapeSQL(item.Title))

	return doltSQLScriptWithRetry(townRoot, script)
}

// ClaimWanted updates a wanted item's status to claimed.
// Returns an error if the item does not exist or is not open.
//
// Uses a single-script approach: UPDATE + DOLT_ADD + DOLT_COMMIT in one
// invocation. If the UPDATE matches 0 rows (item not open), the working set
// is unchanged and DOLT_COMMIT fails with "nothing to commit" — which we
// map to a precondition error. This avoids splitting into separate sessions
// and eliminates the need for DOLT_RESET on failure.
func ClaimWanted(townRoot, wantedID, rigHandle string) error {
	script := fmt.Sprintf(`USE %s;
UPDATE wanted SET claimed_by='%s', status='claimed', updated_at=NOW()
  WHERE id='%s' AND status='open';
CALL DOLT_ADD('-A');
CALL DOLT_COMMIT('-m', 'wl claim: %s');
`, WLCommonsDB, EscapeSQL(rigHandle), EscapeSQL(wantedID), EscapeSQL(wantedID))

	err := doltSQLScriptWithRetry(townRoot, script)
	if err == nil {
		return nil
	}
	if isNothingToCommit(err) {
		return fmt.Errorf("wanted item %q is not open or does not exist", wantedID)
	}
	return fmt.Errorf("claim failed: %w", err)
}

// SubmitCompletion inserts a completion record and updates the wanted status.
// The item must have status='claimed' AND claimed_by=rigHandle to prevent
// completing an item claimed by another rig.
//
// Uses a single-script approach like ClaimWanted. The INSERT uses INSERT IGNORE
// with a SELECT conditional on status='in_review' AND claimed_by AND NOT EXISTS
// (prior completion). INSERT IGNORE makes the script idempotent on retry since
// completions.id is a PRIMARY KEY. NOT EXISTS prevents multiple completions per
// wanted item, ensuring the lifecycle is strictly post→claim→done.
func SubmitCompletion(townRoot, completionID, wantedID, rigHandle, evidence string) error {
	script := fmt.Sprintf(`USE %s;
UPDATE wanted SET status='in_review', evidence_url='%s', updated_at=NOW()
  WHERE id='%s' AND status='claimed' AND claimed_by='%s';
INSERT IGNORE INTO completions (id, wanted_id, completed_by, evidence, completed_at)
  SELECT '%s', '%s', '%s', '%s', NOW()
  FROM wanted WHERE id='%s' AND status='in_review' AND claimed_by='%s'
  AND NOT EXISTS (SELECT 1 FROM completions WHERE wanted_id='%s');
CALL DOLT_ADD('-A');
CALL DOLT_COMMIT('-m', 'wl done: %s');
`,
		WLCommonsDB,
		EscapeSQL(evidence), EscapeSQL(wantedID), EscapeSQL(rigHandle),
		EscapeSQL(completionID), EscapeSQL(wantedID), EscapeSQL(rigHandle), EscapeSQL(evidence),
		EscapeSQL(wantedID), EscapeSQL(rigHandle), EscapeSQL(wantedID),
		EscapeSQL(wantedID))

	err := doltSQLScriptWithRetry(townRoot, script)
	if err == nil {
		return nil
	}
	if isNothingToCommit(err) {
		return fmt.Errorf("wanted item %q is not claimed by %q or does not exist", wantedID, rigHandle)
	}
	return fmt.Errorf("completion failed: %w", err)
}

// QueryWanted fetches a wanted item by ID. Returns nil if not found.
func QueryWanted(townRoot, wantedID string) (*WantedItem, error) {
	query := fmt.Sprintf(`USE %s; SELECT id, title, status, COALESCE(claimed_by, '') as claimed_by FROM wanted WHERE id='%s';`,
		WLCommonsDB, EscapeSQL(wantedID))

	output, err := doltSQLQuery(townRoot, query)
	if err != nil {
		return nil, err
	}

	rows := parseSimpleCSV(output)
	if len(rows) == 0 {
		return nil, fmt.Errorf("wanted item %q not found", wantedID)
	}

	row := rows[0]
	item := &WantedItem{
		ID:        row["id"],
		Title:     row["title"],
		Status:    row["status"],
		ClaimedBy: row["claimed_by"],
	}
	return item, nil
}

// doltSQLQuery executes a SQL query and returns the raw CSV output.
func doltSQLQuery(townRoot, query string) (string, error) {
	config := DefaultConfig(townRoot)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	cmd := buildDoltSQLCmd(ctx, config, "-r", "csv", "-q", query)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("dolt sql query failed: %w (%s)", err, strings.TrimSpace(string(output)))
	}
	return string(output), nil
}

// parseSimpleCSV parses CSV output from dolt sql into a slice of maps.
// Handles quoted fields containing commas and escaped quotes.
func parseSimpleCSV(data string) []map[string]string {
	lines := strings.Split(strings.TrimSpace(data), "\n")
	if len(lines) < 2 {
		return nil
	}

	headers := parseCSVLine(lines[0])
	var result []map[string]string

	for _, line := range lines[1:] {
		if line == "" {
			continue
		}
		fields := parseCSVLine(line)
		row := make(map[string]string)
		for i, h := range headers {
			if i < len(fields) {
				row[strings.TrimSpace(h)] = strings.TrimSpace(fields[i])
			}
		}
		result = append(result, row)
	}
	return result
}

// parseCSVLine parses a single CSV line, handling quoted fields.
func parseCSVLine(line string) []string {
	var fields []string
	var field strings.Builder
	inQuote := false

	for i := 0; i < len(line); i++ {
		ch := line[i]
		switch {
		case ch == '"' && !inQuote:
			inQuote = true
		case ch == '"' && inQuote:
			if i+1 < len(line) && line[i+1] == '"' {
				field.WriteByte('"')
				i++
			} else {
				inQuote = false
			}
		case ch == ',' && !inQuote:
			fields = append(fields, field.String())
			field.Reset()
		default:
			field.WriteByte(ch)
		}
	}
	fields = append(fields, field.String())
	return fields
}
