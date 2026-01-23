// Package beads backend detection and query helpers.
// This file provides functions for detecting the storage backend (SQLite or Dolt)
// and running SQL queries that work with either backend.
package beads

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Backend constants
const (
	BackendSQLite = "sqlite"
	BackendDolt   = "dolt"
)

// MetadataConfig represents the metadata.json configuration file.
type MetadataConfig struct {
	Database          string `json:"database"`
	JSONLExport       string `json:"jsonl_export"`
	Backend           string `json:"backend"`
	Prefix            string `json:"prefix"`
	DoltServerEnabled bool   `json:"dolt_server_enabled"`
	DoltServerHost    string `json:"dolt_server_host"`
	DoltServerPort    int    `json:"dolt_server_port"`
	DoltServerUser    string `json:"dolt_server_user"`
}

// IsDoltServerMode returns true if the beads directory uses Dolt server mode.
// Server mode connects to a centralized dolt sql-server instead of embedded driver.
func IsDoltServerMode(beadsDir string) bool {
	metadataPath := filepath.Join(beadsDir, "metadata.json")
	data, err := os.ReadFile(metadataPath) //nolint:gosec // G304: path is constructed internally
	if err != nil {
		return false
	}

	var config MetadataConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return false
	}

	return config.Backend == BackendDolt && config.DoltServerEnabled
}

// DetectBackend reads metadata.json from the beads directory and returns the backend type.
// Returns "sqlite" by default if no backend is specified or if metadata.json doesn't exist.
func DetectBackend(beadsDir string) string {
	metadataPath := filepath.Join(beadsDir, "metadata.json")
	data, err := os.ReadFile(metadataPath) //nolint:gosec // G304: path is constructed internally
	if err != nil {
		return BackendSQLite // Default to SQLite if no metadata
	}

	var config MetadataConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return BackendSQLite
	}

	if config.Backend == BackendDolt {
		return BackendDolt
	}
	return BackendSQLite
}

// GetDatabasePath returns the path to the database based on the backend type.
// For SQLite, returns path to beads.db.
// For Dolt, returns path to the dolt subdirectory.
func GetDatabasePath(beadsDir string) string {
	backend := DetectBackend(beadsDir)
	if backend == BackendDolt {
		return filepath.Join(beadsDir, "dolt")
	}
	return filepath.Join(beadsDir, "beads.db")
}

// QueryResult represents a row from a SQL query as a map of column names to values.
type QueryResult map[string]interface{}

// RunQuery executes a SQL query against the beads database and returns the results as JSON.
// It automatically detects the backend and uses the appropriate query method.
// The query should be a SELECT statement.
func RunQuery(beadsDir, query string) ([]QueryResult, error) {
	backend := DetectBackend(beadsDir)

	switch backend {
	case BackendDolt:
		return runDoltQuery(beadsDir, query)
	default:
		return runSQLiteQuery(beadsDir, query)
	}
}

// runSQLiteQuery executes a query using sqlite3 CLI.
func runSQLiteQuery(beadsDir, query string) ([]QueryResult, error) {
	dbPath := filepath.Join(beadsDir, "beads.db")

	// Verify database exists
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("SQLite database not found: %s", dbPath)
	}

	cmd := exec.Command("sqlite3", "-json", dbPath, query) //nolint:gosec // G204: query is controlled
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("sqlite3 query failed: %w (%s)", err, strings.TrimSpace(stderr.String()))
	}

	// Handle empty result
	if stdout.Len() == 0 {
		return nil, nil
	}

	var results []QueryResult
	if err := json.Unmarshal(stdout.Bytes(), &results); err != nil {
		return nil, fmt.Errorf("parsing sqlite3 JSON output: %w", err)
	}

	return results, nil
}

// runDoltQuery executes a query using dolt sql CLI.
// Note: dolt sql must be run from within the Dolt database directory.
func runDoltQuery(beadsDir, query string) ([]QueryResult, error) {
	doltPath := filepath.Join(beadsDir, "dolt")

	// Verify dolt directory exists
	if _, err := os.Stat(doltPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("Dolt database not found: %s", doltPath)
	}

	cmd := exec.Command("dolt", "sql", "-q", query, "--result-format", "json") //nolint:gosec // G204: query is controlled
	cmd.Dir = doltPath
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("dolt sql query failed: %w (%s)", err, strings.TrimSpace(stderr.String()))
	}

	// Handle empty result
	if stdout.Len() == 0 {
		return nil, nil
	}

	// Dolt returns JSON in format: {"rows": [...]}
	var doltResult struct {
		Rows []QueryResult `json:"rows"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &doltResult); err != nil {
		return nil, fmt.Errorf("parsing dolt JSON output: %w", err)
	}

	return doltResult.Rows, nil
}

// FindBeadsDBsInRigs searches for beads databases in rig directories.
// It respects the backend configuration in each rig's metadata.json.
// Returns a list of (beadsDir, dbPath) tuples.
func FindBeadsDBsInRigs(townRoot string) ([]string, error) {
	// Discover rigs with beads databases
	rigDirs, err := filepath.Glob(filepath.Join(townRoot, "*", "polecats"))
	if err != nil {
		return nil, fmt.Errorf("finding rig directories: %w", err)
	}

	var beadsDirs []string
	for _, polecatsDir := range rigDirs {
		rigDir := filepath.Dir(polecatsDir)
		beadsDir := filepath.Join(rigDir, "mayor", "rig", ".beads")

		// Check if beads directory exists
		if _, err := os.Stat(beadsDir); err != nil {
			continue
		}

		// Verify the database exists (SQLite or Dolt)
		dbPath := GetDatabasePath(beadsDir)
		if _, err := os.Stat(dbPath); err == nil {
			beadsDirs = append(beadsDirs, beadsDir)
		}
	}

	return beadsDirs, nil
}
