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

// IsDoltNative returns true if beads uses dolt-native mode (shared via symlinks).
// Dolt-native mode is characterized by dolt/.dolt being a symlink to a shared database.
func IsDoltNative(beadsDir string) bool {
	if !IsDoltServerMode(beadsDir) {
		return false
	}
	// Check if dolt/.dolt is a symlink (characteristic of dolt-native)
	doltMetaPath := filepath.Join(beadsDir, "dolt", ".dolt")
	fi, err := os.Lstat(doltMetaPath)
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeSymlink != 0
}

// SetupDoltNativeSymlinks creates symlinks from rig's dolt/ to town's dolt/.
// This allows rigs with tracked .beads/ to share the town-level Dolt database.
// It preserves the rig's config.yaml and formulas but replaces the dolt/ directory
// with symlinks to the town-level database.
func SetupDoltNativeSymlinks(rigBeadsDir, townBeadsDir string) error {
	rigDoltDir := filepath.Join(rigBeadsDir, "dolt")

	// Remove existing dolt/ directory (from tracked beads)
	if err := os.RemoveAll(rigDoltDir); err != nil {
		return fmt.Errorf("removing existing dolt directory: %w", err)
	}

	// Create new dolt/ directory
	if err := os.MkdirAll(rigDoltDir, 0755); err != nil {
		return fmt.Errorf("creating dolt directory: %w", err)
	}

	// Calculate relative path from rig's dolt/ to town's dolt/
	townDoltDir := filepath.Join(townBeadsDir, "dolt")
	relPath, err := filepath.Rel(rigDoltDir, townDoltDir)
	if err != nil {
		return fmt.Errorf("calculating relative path: %w", err)
	}

	// Create symlinks for .dolt, .doltcfg, and beads
	symlinks := []string{".dolt", ".doltcfg", "beads"}
	for _, name := range symlinks {
		linkPath := filepath.Join(rigDoltDir, name)
		target := filepath.Join(relPath, name)
		if err := os.Symlink(target, linkPath); err != nil {
			return fmt.Errorf("creating symlink %s: %w", name, err)
		}
	}

	// Copy dolt server settings from town's metadata.json to rig's metadata.json
	townMetadataPath := filepath.Join(townBeadsDir, "metadata.json")
	townData, err := os.ReadFile(townMetadataPath) //nolint:gosec // G304: path is constructed internally
	if err != nil {
		return fmt.Errorf("reading town metadata: %w", err)
	}

	var townConfig MetadataConfig
	if err := json.Unmarshal(townData, &townConfig); err != nil {
		return fmt.Errorf("parsing town metadata: %w", err)
	}

	// Read rig's existing metadata (if any) or create new
	rigMetadataPath := filepath.Join(rigBeadsDir, "metadata.json")
	var rigConfig MetadataConfig
	if rigData, err := os.ReadFile(rigMetadataPath); err == nil { //nolint:gosec // G304: path is constructed internally
		_ = json.Unmarshal(rigData, &rigConfig)
	}

	// Copy dolt server settings from town
	rigConfig.Backend = townConfig.Backend
	rigConfig.Database = townConfig.Database
	rigConfig.DoltServerEnabled = townConfig.DoltServerEnabled
	rigConfig.DoltServerHost = townConfig.DoltServerHost
	rigConfig.DoltServerPort = townConfig.DoltServerPort
	rigConfig.DoltServerUser = townConfig.DoltServerUser

	// Write updated metadata
	rigData, err := json.MarshalIndent(rigConfig, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling rig metadata: %w", err)
	}
	if err := os.WriteFile(rigMetadataPath, rigData, 0644); err != nil {
		return fmt.Errorf("writing rig metadata: %w", err)
	}

	return nil
}

// GetStorageBackend detects the storage backend from metadata.json or config.yaml.
// Priority order:
// 1. metadata.json "backend" field (takes precedence)
// 2. config.yaml "storage-backend" field
// 3. Default to "sqlite"
func GetStorageBackend(beadsDir string) string {
	// First, check metadata.json (highest priority)
	metadataPath := filepath.Join(beadsDir, "metadata.json")
	if data, err := os.ReadFile(metadataPath); err == nil { //nolint:gosec // G304: path is constructed internally
		var config MetadataConfig
		if err := json.Unmarshal(data, &config); err == nil {
			if config.Backend == BackendDolt {
				return BackendDolt
			}
			if config.Backend == BackendSQLite {
				return BackendSQLite
			}
		}
	}

	// Second, check config.yaml
	configPath := filepath.Join(beadsDir, "config.yaml")
	if data, err := os.ReadFile(configPath); err == nil { //nolint:gosec // G304: path is constructed internally
		// Simple YAML parsing for storage-backend field
		// Format: storage-backend: dolt
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "storage-backend:") {
				value := strings.TrimSpace(strings.TrimPrefix(line, "storage-backend:"))
				if value == BackendDolt {
					return BackendDolt
				}
				if value == BackendSQLite {
					return BackendSQLite
				}
			}
		}
	}

	return BackendSQLite
}

// DetectBackend is an alias for GetStorageBackend for backwards compatibility.
func DetectBackend(beadsDir string) string {
	return GetStorageBackend(beadsDir)
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
	// The Dolt database can be at either:
	// - beadsDir/dolt (simple setup)
	// - beadsDir/dolt/beads (server setup with nested database)
	doltPath := filepath.Join(beadsDir, "dolt")
	nestedPath := filepath.Join(doltPath, "beads")

	// Check for nested database directory first (common in Dolt server setups)
	// The actual database has a .dolt/noms directory
	if _, err := os.Stat(filepath.Join(nestedPath, ".dolt", "noms")); err == nil {
		doltPath = nestedPath
	} else if _, err := os.Stat(filepath.Join(doltPath, ".dolt", "noms")); os.IsNotExist(err) {
		// Neither location has a valid Dolt database
		return nil, fmt.Errorf("Dolt database not found: %s or %s", doltPath, nestedPath)
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
